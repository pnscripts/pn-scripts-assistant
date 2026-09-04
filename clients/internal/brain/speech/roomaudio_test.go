package speech

import (
	"strings"
	"testing"
)

/*
 * Finding the streams to move, and only those.
 *
 * wpctl prints devices and streams in one tree, and moving a device is not the
 * same kind of act as moving a stream — it is how somebody's sound card ends
 * up pointed at itself. The ids look identical, so the section they are under
 * is the only thing that tells them apart.
 *
 * This is real output from the machine this was written on, with a browser
 * playing music, which is the case the whole feature exists for.
 */
const wpctlListing = `PipeWire 'pipewire-0' [1.0.5, petar@cen, cookie:1234]
 └─ Clients:
        32. xdg-desktop-portal                  [1.0.5, petar@cen, pid:2100]
        44. Brave                               [1.0.5, petar@cen, pid:9001]

Audio
 ├─ Devices:
 │      52. Built-in Audio                      [alsa]
 │      58. Trust GXT 232 Microphone            [alsa]
 │
 ├─ Sinks:
 │  *   61. Built-in Audio Analog Stereo        [vol: 0.65]
 │      74. pn_brain_echo_sink                  [vol: 1.00]
 │
 ├─ Sources:
 │  *   66. Trust GXT 232 Microphone            [vol: 1.00]
 │      75. pn_brain_echo_cancelled             [vol: 1.00]
 │
 ├─ Filters:
 │      70. pn-brain.echo-cancel
 │
 └─ Streams:
        88. Brave
            89. output_FL    > Built-in Audio Analog Stereo:playback_FL   [active]
            90. output_FR    > Built-in Audio Analog Stereo:playback_FR   [active]
        95. speech-dispatcher-espeak-ng
            96. output_FL    > Built-in Audio Analog Stereo:playback_FL   [active]

Video
 ├─ Devices:
 │      53. Integrated Camera                   [v4l2]
 │
 └─ Streams:

Settings
 └─ Default Configured Devices:
         0. Audio/Sink    alsa_output.pci-0000_00_1f.3.analog-stereo
`

func TestOnlyTheStreamsAreMovedAndNotTheDevices(t *testing.T) {
	moving := playingStreams(wpctlListing)

	if len(moving) == 0 {
		t.Fatal("nothing was found to move, on a listing with two streams playing")
	}

	found := map[string]bool{}

	for _, id := range moving {
		found[id] = true
	}

	// The two programs playing sound.
	for _, want := range []string{"88", "95"} {
		if !found[want] {
			t.Errorf("stream %s was not going to be moved", want)
		}
	}

	/*
	 * And nothing from any other section.
	 *
	 * Moving a device, a sink or a source is a different act entirely — 61 is
	 * the sound card and 74 is the canceller's own sink, and pointing either
	 * at the default output is how a machine ends up with no sound or with a
	 * loop.
	 */
	for _, mustNot := range []string{"52", "58", "61", "74", "66", "75", "70", "53", "0"} {
		if found[mustNot] {
			t.Errorf("%s is not a stream and would have been moved", mustNot)
		}
	}
}

/*
 * A machine with nothing playing is not an error and not a surprise.
 *
 * The ordinary case: somebody switches this on in a quiet room, and there is
 * nothing to move until they play something.
 */
func TestNothingPlayingIsFine(t *testing.T) {
	quiet := strings.Replace(wpctlListing,
		`        88. Brave
            89. output_FL    > Built-in Audio Analog Stereo:playback_FL   [active]
            90. output_FR    > Built-in Audio Analog Stereo:playback_FR   [active]
        95. speech-dispatcher-espeak-ng
            96. output_FL    > Built-in Audio Analog Stereo:playback_FL   [active]
`, "", 1)

	if moving := playingStreams(quiet); len(moving) != 0 {
		t.Errorf("found %v to move in a listing with no streams", moving)
	}

	if moving := playingStreams(""); len(moving) != 0 {
		t.Errorf("found %v to move in an empty listing", moving)
	}

	if moving := playingStreams("not a listing at all"); len(moving) != 0 {
		t.Errorf("found %v to move in nonsense", moving)
	}
}

/*
 * The video section has streams too, and they are not sound.
 *
 * A camera stream moved to an audio sink is a mistake that would be hard to
 * explain afterwards, and the sections are the only thing separating them.
 */
func TestVideoStreamsAreLeftAlone(t *testing.T) {
	withCamera := strings.Replace(wpctlListing,
		`Video
 ├─ Devices:
 │      53. Integrated Camera                   [v4l2]
 │
 └─ Streams:
`,
		`Video
 ├─ Devices:
 │      53. Integrated Camera                   [v4l2]
 │
 └─ Streams:
        99. Cheese
`, 1)

	for _, id := range playingStreams(withCamera) {
		if id == "99" {
			t.Error("a camera stream was going to be moved to an audio sink")
		}
	}
}
