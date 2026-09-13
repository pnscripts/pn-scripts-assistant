package speech

import (
	"os"
	"strings"
	"testing"
)

/*
 * The machines this will run on, none of which is the one it was written on.
 *
 * Every bug this file guards against presents identically to the person using
 * it — they talk, and nothing happens — and none of them is visible from
 * inside the program without asking the question these tests ask. The real one
 * cost a morning: the echo canceller was subtracting the assistant's voice
 * from an analog jack with nothing plugged into it, while the microphone the
 * person was speaking into sat unused two devices away. Audio still arrived,
 * levels were still measured, recordings were still written and transcribed.
 * They were recordings of an empty room.
 */

// TestPicksTheMicrophoneSomeoneIsSpeakingInto covers the shapes of machine this
// is likely to meet.
func TestPicksTheMicrophoneSomeoneIsSpeakingInto(t *testing.T) {
	for _, c := range []struct {
		machine string
		mics    []Microphone
		want    string
	}{
		{
			// The machine this was written on, and the bug it was written for.
			// The empty analog jack is the desktop's default; the USB
			// microphone is the one being spoken into.
			machine: "USB microphone, empty analog jack marked default",
			mics: []Microphone{
				{ID: "alsa_input.pci-0000_00_1f.3.analog-stereo",
					Name: "Built-in Audio Analog Stereo", Default: true},
				{ID: "alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback",
					Name: "Trust GXT 232 Microphone Mono"},
			},
			want: "alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback",
		},
		{
			machine: "laptop with nothing plugged in",
			mics: []Microphone{
				{ID: "alsa_input.pci-0000_00_1f.3.analog-stereo",
					Name: "Built-in Audio Analog Stereo", Default: true},
			},
			want: "alsa_input.pci-0000_00_1f.3.analog-stereo",
		},
		{
			machine: "laptop with a USB headset",
			mics: []Microphone{
				{ID: "alsa_input.pci-0000_00_1f.3.analog-stereo",
					Name: "Built-in Audio Analog Stereo", Default: true},
				{ID: "alsa_input.usb-Logitech_headset-00.mono-fallback",
					Name: "Logitech Headset"},
			},
			want: "alsa_input.usb-Logitech_headset-00.mono-fallback",
		},
		{
			machine: "bluetooth earbuds beat the built-in microphone",
			mics: []Microphone{
				{ID: "alsa_input.pci-0000_00_1f.3.analog-stereo",
					Name: "Built-in Audio Analog Stereo", Default: true},
				{ID: "bluez_input.AC_12_2F_00_11_22", Name: "WH-1000XM4"},
			},
			want: "bluez_input.AC_12_2F_00_11_22",
		},
		{
			// A monitor's HDMI input is an Audio/Source with a plausible name
			// and nobody has ever spoken into one.
			machine: "desktop whose monitor offers an HDMI input",
			mics: []Microphone{
				{ID: "alsa_input.pci-0000_01_00.1.hdmi-stereo",
					Name: "HDMI / DisplayPort", Default: true},
				{ID: "alsa_input.pci-0000_00_1f.3.analog-stereo",
					Name: "Built-in Audio Analog Stereo"},
			},
			want: "alsa_input.pci-0000_00_1f.3.analog-stereo",
		},
		{
			machine: "webcam with a microphone in it",
			mics: []Microphone{
				{ID: "alsa_input.pci-0000_00_1f.3.analog-stereo",
					Name: "Built-in Audio Analog Stereo", Default: true},
				{ID: "alsa_input.usb-046d_HD_Pro_Webcam_C920-02.analog-stereo",
					Name: "HD Pro Webcam C920 Analog Stereo"},
			},
			want: "alsa_input.usb-046d_HD_Pro_Webcam_C920-02.analog-stereo",
		},
	} {
		t.Run(c.machine, func(t *testing.T) {
			ranked := rankMicrophones(c.mics)
			if len(ranked) == 0 {
				t.Fatalf("%s: ranked nothing at all", c.machine)
			}

			if ranked[0] != c.want {
				t.Errorf("%s:\n  chose %s\n   want %s", c.machine, ranked[0], c.want)
			}
		})
	}
}

// TestNeverRecordsFromTheSpeakers keeps monitors and the canceller's own output
// out of the running.
//
// A monitor is the speakers' output offered back as an input, so recording one
// means the assistant hears whatever is playing — itself, perfectly — and the
// person not at all. Feeding the canceller its own output is the same mistake
// wearing a better name.
func TestNeverRecordsFromTheSpeakers(t *testing.T) {
	mics := []Microphone{
		{ID: "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor", Name: "Monitor of Built-in"},
		{ID: "pn_scripts_assistant_echo_cancelled", Name: "Microphone (echo cancelled)"},
		{ID: "alsa_input.usb-145f_Trust.mono-fallback", Name: "Trust GXT 232"},
	}

	for _, m := range mics[:2] {
		if !notAMicrophone(m.ID) {
			t.Errorf("%s would be offered as something to speak into", m.ID)
		}
	}

	if notAMicrophone(mics[2].ID) {
		t.Errorf("%s is a real microphone and was excluded", mics[2].ID)
	}
}

/*
 * TestNoticesTheCancellerIsOnTheWrongMicrophone is the regression test for the
 * morning that went missing.
 *
 * The canceller was running, the source existed, its name still said echo
 * cancelled, and it was wired to an empty jack. Preferring it by name alone —
 * which is what the code did — meant every turn recorded a faint hiss, crossed
 * the level threshold on noise, and came back from the recogniser with no
 * words. Nothing anywhere said which input it had settled on.
 */
func TestNoticesTheCancellerIsOnTheWrongMicrophone(t *testing.T) {
	const wired = `pn-scripts-assistant.echo-cancel.capture:input_MONO
  |<- alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback:capture_MONO
pn_scripts_assistant_echo_cancelled:capture_MONO
  |-> pw-record:input_MONO
`

	// Under the old name, as a machine whose audio has not been restarted
	// since the rename still lists it.
	const misdirected = `pn-brain.echo-cancel.capture:input_FL
  |<- alsa_input.pci-0000_00_1f.3.analog-stereo:capture_FL
pn-brain.echo-cancel.capture:input_FR
  |<- alsa_input.pci-0000_00_1f.3.analog-stereo:capture_FR
`

	if got := whatFeedsCanceller(wired); got != "alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback" {
		t.Errorf("mono microphone: read the source as %q", got)
	}

	// Stereo, so the port is input_FL rather than input_MONO. Matching the
	// whole port name instead of the prefix would miss this and report no
	// source at all, which reads as "no canceller" and hides the fault.
	if got := whatFeedsCanceller(misdirected); got != "alsa_input.pci-0000_00_1f.3.analog-stereo" {
		t.Errorf("stereo built-in: read the source as %q", got)
	}

	if got := whatFeedsCanceller("no canceller here\n"); got != "" {
		t.Errorf("no canceller running: read the source as %q", got)
	}
}

// TestTheCancellerConfigNamesAMicrophone guards the one line whose absence
// caused all of this.
func TestTheCancellerConfigNamesAMicrophone(t *testing.T) {
	const mic = "alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback"

	body := echoCancelText(mic)

	if !strings.Contains(body, "target.object") {
		t.Fatal("the config does not say which microphone to capture from, " +
			"so PipeWire will pick the default input — which is how the " +
			"canceller ended up on an empty jack")
	}

	if !strings.Contains(body, mic) {
		t.Errorf("the config does not name %s", mic)
	}
}

// TestListsMicrophonesWithoutPipeWire covers machines that never had it: a
// minimal install, a server, a distribution that stayed with PulseAudio.
func TestListsMicrophonesWithoutPipeWire(t *testing.T) {
	// Left as arecord prints it, trailing spaces and all.
	const listing = `**** List of CAPTURE Hardware Devices ****
card 0: PCH [HDA Intel PCH], device 0: ALC3246 Analog [ALC3246 Analog]
  Subdevices: 1/1
  Subdevice #0: subdevice #0
card 1: Microphone [Trust GXT 232 Microphone], device 0: USB Audio [USB Audio]
  Subdevices: 1/1
  Subdevice #0: subdevice #0
`

	var found []Microphone

	for _, line := range strings.Split(listing, "\n") {
		match := alsaCard.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		found = append(found, Microphone{
			ID:   "hw:" + match[1] + "," + match[3],
			Name: match[2],
		})
	}

	if len(found) != 2 {
		t.Fatalf("read %d cards from arecord -l, want 2: %+v", len(found), found)
	}

	if found[1].ID != "hw:1,0" || found[1].Name != "Trust GXT 232 Microphone" {
		t.Errorf("second card came back as %+v", found[1])
	}
}

/*
 * TestNeverHandsAPipeWireNameToALSA covers the machine with no PipeWire that
 * still has a PipeWire device name saved in its settings.
 *
 * The two name devices in entirely different ways. arecord answers a
 * PipeWire node name with a message about an unknown PCM, which to anyone who
 * has not seen it before reads as the microphone being broken.
 */
func TestNeverHandsAPipeWireNameToALSA(t *testing.T) {
	for _, id := range []string{
		"alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback",
		"pn_scripts_assistant_echo_cancelled",
	} {
		if alsaWouldUnderstand(id) {
			t.Errorf("%q is a PipeWire name and would be passed to arecord", id)
		}
	}

	for _, id := range []string{"hw:1,0", "plughw:0,0", "default"} {
		if !alsaWouldUnderstand(id) {
			t.Errorf("%q is an ALSA name and would be dropped", id)
		}
	}
}

// TestReadsTheMicrophoneOutOfTheConfigNotTheComments guards a parser that
// matched its own explanation.
//
// The config file explains what target.object is for, and reading a comment
// back reported the chosen microphone as "names the microphone explicitly" —
// which is not a device, and which the interface would have shown as the input
// in use.
func TestReadsTheMicrophoneOutOfTheConfigNotTheComments(t *testing.T) {
	const mic = "alsa_input.usb-145f_Trust_GXT_232_Microphone-00.mono-fallback"

	body := echoCancelText(mic)

	var found string

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		if _, value, ok := strings.Cut(line, "target.object"); ok {
			found = strings.Trim(strings.TrimSpace(strings.TrimPrefix(
				strings.TrimSpace(value), "=")), `"`)

			break
		}
	}

	if found != mic {
		t.Errorf("read the microphone as %q, want %q", found, mic)
	}
}

/*
 * TestBothWaysOfListeningChooseAnInput guards the two recording paths against
 * drifting apart again.
 *
 * They did. Conversation mode learned to pick the best microphone and one-shot
 * listening did not, so on the same machine holding the button recorded from
 * an analog jack with nothing plugged into it while speaking a turn recorded
 * from the USB microphone. Both reported a level, both wrote a file, and only
 * one of them contained anybody — which reads as the button being broken.
 */
func TestBothWaysOfListeningChooseAnInput(t *testing.T) {
	for _, source := range []string{"listen.go", "vad.go"} {
		body, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("could not read %s: %v", source, err)
		}

		if !strings.Contains(string(body), "device = PreferredMicrophone(ctx)") {
			t.Errorf("%s does not choose a microphone when none is given, so it "+
				"falls back to the desktop default — which is often an empty jack",
				source)
		}
	}
}
