package speech

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

/*
 * Setting up echo cancellation for the microphone actually in use.
 *
 * Without it the microphone hears the assistant's own voice off the speakers,
 * so the listening loop has to stop whenever it speaks — and a loop that is not
 * listening cannot be interrupted. With it, both can happen at once, the way
 * they do on a telephone.
 *
 * The part that has to be right is which microphone it captures from. Left
 * unsaid, PipeWire links the canceller to whatever it considers the default
 * input, and on this machine that is the built-in jack with nothing plugged
 * into it. The result is an assistant that hears a faint hiss instead of the
 * person in front of it — measured here as a noise floor of 155 against a voice
 * reaching 296, where the real microphone gives a floor of 20 and peaks near
 * 2000. Nothing in the interface says which input it settled on, so it reads as
 * the microphone having stopped working.
 */

// echoCancelConfig is where the file goes: the owner's own PipeWire settings,
// so none of this needs a password and deleting the file undoes all of it.
func echoCancelConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".config", "pipewire", "pipewire.conf.d",
		"99-pn-brain-echo-cancel.conf"), nil
}

/*
 * SetUpEchoCancellation points the canceller at one microphone.
 *
 * Rewritten and reloaded whenever the chosen microphone changes, because a
 * canceller aimed at the wrong input is worse than none at all: it is quieter,
 * noisier, and gives no sign of which input it took.
 */
func SetUpEchoCancellation(ctx context.Context, microphone string) error {
	if strings.TrimSpace(microphone) == "" {
		return fmt.Errorf("say which microphone to cancel the echo from")
	}

	// Its own name would make it cancel the echo of its own output.
	if strings.Contains(microphone, "echo") {
		return fmt.Errorf("that is already the echo-cancelled input")
	}

	path, err := echoCancelConfig()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	body := echoCancelText(microphone)

	// Unchanged means nothing to restart, and restarting audio interrupts
	// whatever is playing.
	if existing, err := os.ReadFile(path); err == nil && string(existing) == body {
		return nil
	}

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}

	return reloadAudio(ctx)
}

// EchoCancellationOn reports whether it is set up, and for which microphone.
func EchoCancellationOn() (bool, string) {
	path, err := echoCancelConfig()
	if err != nil {
		return false, ""
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)

		// The comments in this file explain what target.object is for, and
		// reading one of those back reported the microphone as "names the
		// microphone explicitly" — which is not a device anyone can record
		// from, and which the interface would have shown as the chosen input.
		if strings.HasPrefix(line, "#") {
			continue
		}

		if _, value, found := strings.Cut(line, "target.object"); found {
			return true, strings.Trim(strings.TrimSpace(strings.TrimPrefix(
				strings.TrimSpace(value), "=")), `"`)
		}
	}

	return true, ""
}

// TurnOffEchoCancellation removes the file and puts the audio back.
func TurnOffEchoCancellation(ctx context.Context) error {
	path, err := echoCancelConfig()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return reloadAudio(ctx)
}

func echoCancelText(microphone string) string {
	return `# Echo cancellation, so the brain does not hear itself.
#
# Written by PN Brain, and rewritten when the chosen microphone changes.
# Deleting this file and restarting PipeWire undoes it completely.
#
# target.object names the microphone explicitly, and that is the whole of the
# difference between this working and not: left unsaid, the canceller is linked
# to whatever PipeWire considers the default input, which is often a built-in
# jack with nothing plugged into it.
context.modules = [
    {   name = libpipewire-module-echo-cancel
        args = {
            capture.props = {
                node.name     = "pn-brain.echo-cancel.capture"
                target.object = "` + microphone + `"
                node.passive  = true
            }
            source.props = {
                node.name        = "pn_brain_echo_cancelled"
                node.description = "Microphone (echo cancelled)"
            }
            playback.props = {
                node.name    = "pn-brain.echo-cancel.playback"
                node.passive = true
            }
            sink.props = {
                node.name        = "pn_brain_echo_sink"
                node.description = "Speakers (echo cancelled)"
            }
        }
    }
]
`
}

// reloadAudio restarts the user's own audio services, which needs no password.
func reloadAudio(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "restart",
		"pipewire", "pipewire-pulse", "wireplumber").CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not restart audio: %s", strings.TrimSpace(string(out)))
	}

	return nil
}
