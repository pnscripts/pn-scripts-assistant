package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Microphone is an input the brain could listen through.
//
// ID is PipeWire's node.name, not its object id. They are different numbers for
// the same device — the USB microphone here is object 33 and serial 50 — and
// pw-record's --target reads a serial or a name, never an object id. Passing
// the id silently targeted a serial that did not exist, so recording fell back
// to the default input and every attempt captured the wrong device. The name is
// unambiguous and survives reconnection.
type Microphone struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// Microphones lists the audio inputs on this machine.
//
// Asked of PipeWire rather than ALSA. On a modern desktop ALSA's "default"
// capture device is whatever PipeWire put there, which on this machine is the
// built-in analog jack — so recording from it produces near-silence while a USB
// microphone sits unused two devices away. That failure looks exactly like a
// broken recogniser, which is the wrong thing to go debugging.
func Microphones(ctx context.Context) ([]Microphone, error) {
	if _, err := exec.LookPath("pw-dump"); err != nil {
		return nil, fmt.Errorf("PipeWire is not available, so inputs cannot be listed")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil, fmt.Errorf("could not ask PipeWire for inputs: %w", err)
	}

	var objects []struct {
		ID   int `json:"id"`
		Info struct {
			Props map[string]any `json:"props"`
		} `json:"info"`
	}

	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, fmt.Errorf("could not read PipeWire's reply: %w", err)
	}

	defaultName := defaultSourceName(ctx)

	var out []Microphone

	for _, o := range objects {
		props := o.Info.Props

		if str(props["media.class"]) != "Audio/Source" {
			continue
		}

		nodeName := str(props["node.name"])
		if nodeName == "" {
			continue
		}

		label := str(props["node.description"])
		if label == "" {
			label = nodeName
		}

		out = append(out, Microphone{
			ID:      nodeName,
			Name:    label,
			Default: defaultName != "" && nodeName == defaultName,
		})
	}

	return out, nil
}

// defaultSourceName asks which input the desktop currently prefers.
//
// Reported rather than obeyed: the default is frequently wrong — an empty
// analog jack outranking a plugged-in USB microphone — so it is shown as a
// label and the choice is left to the person.
func defaultSourceName(ctx context.Context) string {
	if _, err := exec.LookPath("wpctl"); err != nil {
		return ""
	}

	raw, err := exec.CommandContext(ctx, "wpctl", "inspect", "@DEFAULT_AUDIO_SOURCE@").Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "node.name") {
			continue
		}

		if _, value, found := strings.Cut(line, "="); found {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}

	return ""
}

func str(v any) string {
	s, _ := v.(string)

	return s
}

/*
 * PreferredMicrophone is the input to record from when nobody has chosen one.
 *
 * An echo-cancelled source wins over anything else, and it is the difference
 * between an assistant that can be interrupted and one that cannot. A
 * microphone in front of a speaker hears the assistant's own voice, so the
 * listening loop had to stop whenever it spoke — and a loop that is not
 * listening cannot be interrupted. With the assistant's own output subtracted,
 * it can listen and speak at the same time, the way a person on a telephone
 * does.
 *
 * Returns empty when there is no such source, and then whatever was configured
 * or is the system default is used as before.
 */
func PreferredMicrophone(ctx context.Context) string {
	mics, err := Microphones(ctx)
	if err != nil {
		return ""
	}

	for _, m := range mics {
		// By name, set in the config this program writes, rather than by the
		// description — which is translated on a machine in another language
		// and would silently stop matching.
		if strings.Contains(m.ID, "echo_cancelled") || strings.Contains(m.ID, "echo-cancel") {
			return m.ID
		}
	}

	return ""
}
