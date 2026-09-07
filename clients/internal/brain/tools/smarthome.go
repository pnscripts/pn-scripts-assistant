package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/smarthome"
)

// ListDevices reports what is in the house and what state it is in.
type ListDevices struct {
	Home *smarthome.HomeAssistant
}

func (ListDevices) Name() string { return "list_devices" }

func (ListDevices) Description() string {
	return "List the smart-home devices and their current state."
}

func (ListDevices) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (ListDevices) Risk() Risk { return Safe }

func (ListDevices) Summarize(json.RawMessage) string { return "List smart-home devices" }

func (l ListDevices) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	devices, err := l.Home.Devices(ctx)
	if err != nil {
		return "", err
	}

	if len(devices) == 0 {
		return "No devices found.", nil
	}

	var b strings.Builder

	for _, d := range devices {
		fmt.Fprintf(&b, "%s — %s: %s", d.ID, d.Name, d.State)

		if d.Unit != "" {
			b.WriteString(" " + d.Unit)
		}

		b.WriteString("\n")
	}

	return b.String(), nil
}

// SetDevice turns something on or off.
//
// Mutating, and not as a formality. This is the only tool whose effect is
// physical: a light actually comes on, a lock actually opens. Undoing a bad
// file write is a matter of restoring a backup; undoing an unlocked door at
// three in the morning is not. It stops and asks, every time.
type SetDevice struct {
	Home *smarthome.HomeAssistant
}

func (SetDevice) Name() string { return "set_device" }

func (SetDevice) Description() string {
	return "Turn a smart-home device on or off, or toggle it. Requires the owner's approval."
}

func (SetDevice) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Entity id, e.g. light.kitchen"},
			"state": {"type": "string", "description": "on, off or toggle"}
		},
		"required": ["id", "state"]
	}`)
}

func (SetDevice) Risk() Risk { return Mutating }

type deviceArgs struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// Summarize names the device and what will happen to it, because "adjust a
// device" tells the person approving nothing they need.
func (SetDevice) Summarize(raw json.RawMessage) string {
	var a deviceArgs
	argsOf(raw, &a)

	verb := strings.ToLower(strings.TrimSpace(a.State))

	// A lock is worth calling out by name. Approving "set lock.front to off"
	// should not be how somebody discovers they unlocked their front door.
	if strings.HasPrefix(a.ID, "lock.") {
		switch verb {
		case "off", "lock":
			return "LOCK " + a.ID
		case "on", "unlock":
			return "UNLOCK " + a.ID + " — this physically unlocks a door"
		}
	}

	return fmt.Sprintf("Set %s to %s", a.ID, a.State)
}

func (s SetDevice) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a deviceArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := s.Home.SetState(ctx, a.ID, a.State); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s is now %s.", a.ID, a.State), nil
}
