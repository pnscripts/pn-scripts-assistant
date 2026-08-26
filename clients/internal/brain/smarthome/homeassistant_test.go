package smarthome

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T, handler http.HandlerFunc) *HomeAssistant {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return New(ts.URL, "a-token")
}

// Most people do not run Home Assistant. Being unconfigured is a normal state,
// and the brain must not present a capability it does not have.
func TestUnconfiguredIsANormalState(t *testing.T) {
	for _, h := range []*HomeAssistant{
		New("", ""),
		New("http://ha.local", ""),
		New("", "token"),
	} {
		if h.Configured() {
			t.Errorf("reported configured with url=%q token=%q", h.BaseURL, h.Token)
		}
	}

	if !New("http://ha.local", "token").Configured() {
		t.Error("a fully configured instance reported otherwise")
	}
}

func TestUnconfiguredExplainsWhatIsMissing(t *testing.T) {
	_, err := New("", "").Devices(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}

	for _, want := range []string{"HOME_ASSISTANT_URL", "HOME_ASSISTANT_TOKEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}
}

// Home Assistant publishes a great deal that is not a controllable device.
func TestOnlyControllableDomainsAreListed(t *testing.T) {
	h := fake(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"entity_id": "light.kitchen", "state": "on",
				"attributes": map[string]any{"friendly_name": "Kitchen"}},
			{"entity_id": "sensor.temperature", "state": "21",
				"attributes": map[string]any{"friendly_name": "Hall", "unit_of_measurement": "°C"}},
			{"entity_id": "lock.front", "state": "locked", "attributes": map[string]any{}},
			// Not devices.
			{"entity_id": "automation.morning", "state": "on", "attributes": map[string]any{}},
			{"entity_id": "update.ha_core", "state": "off", "attributes": map[string]any{}},
			{"entity_id": "person.petar", "state": "home", "attributes": map[string]any{}},
		})
	})

	devices, err := h.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(devices) != 3 {
		t.Fatalf("listed %d devices, want 3: %+v", len(devices), devices)
	}

	byID := map[string]Device{}
	for _, d := range devices {
		byID[d.ID] = d
	}

	if byID["sensor.temperature"].Unit != "°C" {
		t.Error("unit was dropped")
	}

	// An entity with no friendly name still needs something to call it.
	if byID["lock.front"].Name != "lock.front" {
		t.Errorf("unnamed entity became %q", byID["lock.front"].Name)
	}
}

// Home Assistant is service-oriented: you call light.turn_on, you do not assign
// "on". Locks and covers have their own verbs and reject turn_on entirely.
func TestStateIsTranslatedToTheRightService(t *testing.T) {
	cases := []struct {
		entity, desired, wantPath string
	}{
		{"light.kitchen", "on", "/api/services/light/turn_on"},
		{"light.kitchen", "off", "/api/services/light/turn_off"},
		{"switch.pump", "toggle", "/api/services/switch/toggle"},
		{"lock.front", "off", "/api/services/lock/lock"},
		{"lock.front", "on", "/api/services/lock/unlock"},
		{"lock.front", "unlock", "/api/services/lock/unlock"},
		{"cover.garage", "open", "/api/services/cover/open_cover"},
		{"cover.garage", "close", "/api/services/cover/close_cover"},
	}

	for _, c := range cases {
		t.Run(c.entity+"="+c.desired, func(t *testing.T) {
			var got string

			h := fake(t, func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Path
				w.Write([]byte("{}"))
			})

			if err := h.SetState(context.Background(), c.entity, c.desired); err != nil {
				t.Fatal(err)
			}

			if got != c.wantPath {
				t.Errorf("called %s, want %s", got, c.wantPath)
			}
		})
	}
}

func TestNonsenseStatesAreRefused(t *testing.T) {
	h := fake(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a nonsense state reached Home Assistant")
	})

	for _, s := range []string{"brighter", "maybe", "42", ""} {
		if err := h.SetState(context.Background(), "light.kitchen", s); err == nil {
			t.Errorf("accepted state %q", s)
		}
	}

	// An id that is not an entity id has no domain to call a service on.
	if err := h.SetState(context.Background(), "kitchen", "on"); err == nil {
		t.Error("accepted an id with no domain")
	}
}

func TestExpiredTokenSaysSo(t *testing.T) {
	h := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := h.Devices(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("an expired token produced: %v", err)
	}
}

func TestTokenIsSentAsBearer(t *testing.T) {
	var auth string

	h := fake(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte("[]"))
	})

	h.Devices(context.Background())

	if auth != "Bearer a-token" {
		t.Errorf("Authorization was %q", auth)
	}
}
