package speech

import (
	"context"
	"strings"
)

/*
 * The names this program gives its own sound, under both of its names.
 *
 * Every node it makes in the audio graph — the canceller's capture, source,
 * playback and sink — is named after the program, and code all over this
 * package asks "is that one ours". The program was PN Brain and its nodes said
 * so. A machine keeps running the canceller it was given until the config is
 * rewritten and the audio restarted, so for a while the old names and the new
 * ones are both real: asking for only one of them would have the brain turn
 * down its own voice, or route the room through a sink it could not find.
 */
const (
	CaptureNode     = "pn-scripts-assistant.echo-cancel.capture"
	PlaybackNode    = "pn-scripts-assistant.echo-cancel.playback"
	CancelledSource = "pn_scripts_assistant_echo_cancelled"

	// EchoSink is the sink the canceller listens to.
	EchoSink = "pn_scripts_assistant_echo_sink"

	legacyCaptureNode = "pn-brain.echo-cancel.capture"
	legacyEchoSink    = "pn_brain_echo_sink"
)

// ownPrefixes are what every node and stream this program makes is called
// after, now and before the rename.
var ownPrefixes = []string{"pn-scripts-assistant", "pn_scripts_assistant", "pn-brain", "pn_brain"}

// isOwnAudio reports whether a node or stream is named as this program's own.
func isOwnAudio(name string) bool {
	folded := strings.ToLower(name)

	for _, prefix := range ownPrefixes {
		if strings.Contains(folded, prefix) {
			return true
		}
	}

	return false
}

// isEchoSink reports whether a sink is the canceller's, under either name.
func isEchoSink(name string) bool { return name == EchoSink || name == legacyEchoSink }

// isCaptureNode reports whether a node is the canceller's capture, under
// either name.
func isCaptureNode(name string) bool { return name == CaptureNode || name == legacyCaptureNode }

// runningEchoSink is the canceller's sink as it is running now: the new name,
// or the old one on a machine whose audio has not been restarted since the
// rename. Empty when neither is running.
func runningEchoSink(ctx context.Context) string {
	for _, name := range []string{EchoSink, legacyEchoSink} {
		if sinkExists(ctx, name) {
			return name
		}
	}

	return ""
}
