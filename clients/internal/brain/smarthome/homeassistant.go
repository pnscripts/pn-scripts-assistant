// Package smarthome connects the brain to the physical world.
//
// Home Assistant is implemented first because it is the one that pays for
// itself twice: it is open and self-hosted, and it already speaks Zigbee,
// Z-Wave, Tuya, Hue and hundreds of others. Supporting it supports most of
// them, so buying hardware Home Assistant handles is usually wiser than
// writing a per-vendor adapter here.
package smarthome

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Domains worth exposing.
//
// Home Assistant publishes a great deal that is not a controllable device —
// update entities, automation states, diagnostic counters. Restricting to these
// keeps the model's view small and relevant.
var Domains = map[string]bool{
	"light": true, "switch": true, "climate": true, "sensor": true,
	"binary_sensor": true, "cover": true, "fan": true, "lock": true,
}

// Device is one thing in the house.
type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
	State  string `json:"state"`
	Unit   string `json:"unit,omitempty"`
}

// HomeAssistant talks to a Home Assistant instance.
type HomeAssistant struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func New(baseURL, token string) *HomeAssistant {
	return &HomeAssistant{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *HomeAssistant) Name() string { return "home-assistant" }

// Configured reports whether there is anything to talk to.
//
// Being unconfigured is a normal state, not an error: most people do not run
// Home Assistant, and the brain must not present a capability it does not have.
func (h *HomeAssistant) Configured() bool {
	return h != nil && h.BaseURL != "" && h.Token != ""
}

type state struct {
	EntityID   string `json:"entity_id"`
	State      string `json:"state"`
	Attributes struct {
		FriendlyName      string `json:"friendly_name"`
		UnitOfMeasurement string `json:"unit_of_measurement"`
	} `json:"attributes"`
}

// Devices lists what can be seen or controlled.
func (h *HomeAssistant) Devices(ctx context.Context) ([]Device, error) {
	var states []state

	if err := h.call(ctx, http.MethodGet, "/api/states", nil, &states); err != nil {
		return nil, err
	}

	out := make([]Device, 0, len(states))

	for _, s := range states {
		domain, _, found := strings.Cut(s.EntityID, ".")

		if !found || !Domains[domain] {
			continue
		}

		name := s.Attributes.FriendlyName
		if name == "" {
			name = s.EntityID
		}

		out = append(out, Device{
			ID:     s.EntityID,
			Name:   name,
			Domain: domain,
			State:  s.State,
			Unit:   s.Attributes.UnitOfMeasurement,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}

		return out[i].Name < out[j].Name
	})

	return out, nil
}

// SetState turns something on or off.
//
// Home Assistant is service-oriented rather than state-oriented: you call
// light.turn_on, you do not assign "on". Translating here keeps that detail out
// of the tool and out of the model's way.
func (h *HomeAssistant) SetState(ctx context.Context, entityID, desired string) error {
	domain, _, found := strings.Cut(entityID, ".")
	if !found {
		return fmt.Errorf("%q is not an entity id; they look like light.kitchen", entityID)
	}

	var service string

	switch strings.ToLower(strings.TrimSpace(desired)) {
	case "on", "open", "unlock":
		service = "turn_on"
	case "off", "close", "lock":
		service = "turn_off"
	case "toggle":
		service = "toggle"
	default:
		return fmt.Errorf("cannot set %q to %q; use on, off or toggle", entityID, desired)
	}

	// Locks and covers have their own verbs; turn_on would be rejected.
	switch domain {
	case "lock":
		service = map[string]string{"turn_on": "unlock", "turn_off": "lock"}[service]
	case "cover":
		service = map[string]string{"turn_on": "open_cover", "turn_off": "close_cover", "toggle": "toggle"}[service]
	}

	if service == "" {
		return fmt.Errorf("%q cannot be set that way", entityID)
	}

	body := map[string]string{"entity_id": entityID}

	return h.call(ctx, http.MethodPost, "/api/services/"+domain+"/"+service, body, nil)
}

func (h *HomeAssistant) call(ctx context.Context, method, path string, body, into any) error {
	if !h.Configured() {
		return fmt.Errorf(
			"Home Assistant is not configured; set HOME_ASSISTANT_URL and " +
				"HOME_ASSISTANT_TOKEN (a long-lived access token from your profile page)")
	}

	var payload []byte

	if body != nil {
		var err error

		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+h.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Home Assistant at %s: %w", h.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Home Assistant rejected the token; it may have expired")
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Home Assistant returned %d for %s", resp.StatusCode, path)
	}

	if into == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(into)
}
