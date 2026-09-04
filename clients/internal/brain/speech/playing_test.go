package speech

import (
	"strings"
	"testing"
)

/*
 * Whether this machine is making a noise while somebody talks.
 *
 * This is the question behind the complaint that started it: music playing,
 * somebody talking over it, and an assistant answering the music. Getting the
 * answer wrong in either direction is its own kind of wrong — missing the
 * music leaves the confusion unexplained, and inventing music that is not
 * there blames the room for a fault somewhere else.
 *
 * The listings below are real ones from this machine, trimmed. The states in
 * them are the real states too: a browser with a video actually playing sits
 * at "running", and the text-to-speech daemons sit at "idle" from the moment
 * they start until the end of the session.
 */

const realDump = `[
{"id":40,"info":{"state":"running","props":{
  "media.class":"Stream/Output/Audio","node.name":"pn-brain.echo-cancel.playback"}}},
{"id":86,"info":{"state":"idle","props":{
  "media.class":"Stream/Output/Audio","application.name":"speech-dispatcher-dummy",
  "node.name":"speech-dispatcher-dummy"}}},
{"id":95,"info":{"state":"idle","props":{
  "media.class":"Stream/Output/Audio","application.name":"speech-dispatcher-espeak-ng",
  "node.name":"speech-dispatcher-espeak-ng"}}},
{"id":128,"info":{"state":"running","props":{
  "media.class":"Stream/Output/Audio","application.name":"Brave","node.name":"Brave"}}},
{"id":55,"info":{"state":"running","props":{
  "media.class":"Audio/Sink","node.name":"alsa_output.pci-0000_00_1f.3.analog-stereo"}}},
{"id":56,"info":{"state":"running","props":{
  "media.class":"Audio/Source","node.name":"alsa_input.usb-145f_Trust_GXT_232_Microphone"}}},
{"id":38,"info":{"state":"running","props":{
  "media.class":"Audio/Source","node.name":"pn_brain_echo_cancelled"}}}
]`

func TestTheBrowserPlayingMusicIsFoundAndNothingElseIs(t *testing.T) {
	playing, err := playingFromDump([]byte(realDump))
	if err != nil {
		t.Fatalf("reading a real listing: %v", err)
	}

	if len(playing) != 1 || playing[0] != "Brave" {
		t.Fatalf("expected only the browser to be playing, got %v", playing)
	}
}

// The brain's own voice is not something the room is playing at it. It goes
// through the canceller and comes back out of the microphone subtracted.
func TestOurOwnVoiceIsNeverCountedAsSomethingPlaying(t *testing.T) {
	for _, name := range []string{
		"pn-brain.echo-cancel.playback",
		"pn_brain_echo_sink",
		"PN-Brain",
		"piper",
		"pw-play",
		"speech-dispatcher-espeak-ng",
	} {
		dump := `[{"id":1,"info":{"state":"running","props":{
			"media.class":"Stream/Output/Audio","node.name":"` + name + `"}}}]`

		playing, err := playingFromDump([]byte(dump))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if len(playing) != 0 {
			t.Errorf("%s was counted as the machine playing something: %v", name, playing)
		}
	}
}

/*
 * A paused video keeps its stream open.
 *
 * This is the case the plain listing cannot see, and the reason for reading a
 * dump at all. Somebody pauses the film to talk; if the pause still counted,
 * every answer for the rest of the evening would come with "the film was
 * playing" attached to it.
 */
func TestAPausedStreamIsNotPlaying(t *testing.T) {
	for _, state := range []string{"idle", "suspended", "creating", "error"} {
		dump := `[{"id":1,"info":{"state":"` + state + `","props":{
			"media.class":"Stream/Output/Audio","application.name":"Brave"}}}]`

		playing, err := playingFromDump([]byte(dump))
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}

		if len(playing) != 0 {
			t.Errorf("a %s stream was reported as playing: %v", state, playing)
		}
	}
}

// Two tabs, two windows, two streams, one name. Saying "Brave and Brave was
// playing" would be an answer written by a machine.
func TestTheSameProgramTwiceIsNamedOnce(t *testing.T) {
	dump := `[
	{"id":1,"info":{"state":"running","props":{
	  "media.class":"Stream/Output/Audio","application.name":"Brave"}}},
	{"id":2,"info":{"state":"running","props":{
	  "media.class":"Stream/Output/Audio","application.name":"Brave"}}},
	{"id":3,"info":{"state":"running","props":{
	  "media.class":"Stream/Output/Audio","application.name":"Spotify"}}}]`

	playing, err := playingFromDump([]byte(dump))
	if err != nil {
		t.Fatal(err)
	}

	if len(playing) != 2 || playing[0] != "Brave" || playing[1] != "Spotify" {
		t.Fatalf("expected the browser once and the music player once, got %v", playing)
	}
}

// A stream that never named itself still has to be describable, because "one
// of your programs" is a better answer than silence.
func TestAStreamWithNoApplicationNameFallsBackToItsNodeName(t *testing.T) {
	dump := `[{"id":1,"info":{"state":"running","props":{
		"media.class":"Stream/Output/Audio","node.name":"mpv"}}}]`

	playing, err := playingFromDump([]byte(dump))
	if err != nil {
		t.Fatal(err)
	}

	if len(playing) != 1 || playing[0] != "mpv" {
		t.Fatalf("expected the node name to stand in, got %v", playing)
	}
}

// A microphone and a pair of speakers are not programs playing sound.
func TestDevicesAreNotProgramsPlayingSound(t *testing.T) {
	dump := `[
	{"id":1,"info":{"state":"running","props":{
	  "media.class":"Audio/Sink","node.name":"alsa_output.analog-stereo"}}},
	{"id":2,"info":{"state":"running","props":{
	  "media.class":"Audio/Source","node.name":"alsa_input.usb"}}},
	{"id":3,"info":{"state":"running","props":{
	  "media.class":"Stream/Input/Audio","application.name":"pn-brain"}}},
	{"id":4,"info":{"state":"running","props":{
	  "media.class":"Video/Source","node.name":"webcam"}}}]`

	playing, err := playingFromDump([]byte(dump))
	if err != nil {
		t.Fatal(err)
	}

	if len(playing) != 0 {
		t.Fatalf("devices were counted as programs: %v", playing)
	}
}

// A machine with no sound at all — an ai-server over ssh, a laptop with
// everything shut. The question still gets asked there.
func TestAnEmptyMachinePlaysNothing(t *testing.T) {
	for _, dump := range []string{"[]", `[{"id":1,"info":{"props":{}}}]`, `[{"id":1}]`} {
		playing, err := playingFromDump([]byte(dump))
		if err != nil {
			t.Fatalf("%s: %v", dump, err)
		}

		if len(playing) != 0 {
			t.Errorf("%s: found something playing on a silent machine: %v", dump, playing)
		}
	}
}

// Nonsense from a broken pw-dump is reported as such, so the caller can fall
// back to the listing rather than quietly answering "nothing is playing".
func TestRubbishIsAnErrorAndNotAnAnswer(t *testing.T) {
	for _, dump := range []string{"", "not json", "{}", "null\n"} {
		if _, err := playingFromDump([]byte(dump)); err == nil && dump != "null\n" {
			t.Errorf("%q was accepted as a listing", dump)
		}
	}
}

/*
 * The fallback path, for a machine that has pw-cli but not pw-dump.
 *
 * It reads the same facts out of the text form, minus the state it cannot
 * see. This listing is a real one from this machine.
 */
const realListing = `	id 37, type PipeWire:Interface:Node/3
 		object.serial = "37"
 		node.description = "Echo-Cancel Capture"
 		node.name = "pn-brain.echo-cancel.capture"
 		media.class = "Stream/Input/Audio"
	id 39, type PipeWire:Interface:Node/3
 		node.description = "Speakers (echo cancelled)"
 		node.name = "pn_brain_echo_sink"
 		media.class = "Audio/Sink"
	id 40, type PipeWire:Interface:Node/3
 		node.description = "Echo-Cancel Playback"
 		node.name = "pn-brain.echo-cancel.playback"
 		media.class = "Stream/Output/Audio"
	id 95, type PipeWire:Interface:Node/3
 		application.name = "speech-dispatcher-espeak-ng"
 		node.name = "speech-dispatcher-espeak-ng"
 		media.class = "Stream/Output/Audio"
	id 128, type PipeWire:Interface:Node/3
 		application.name = "Brave"
 		media.name = "Playback"
 		node.name = "Brave"
 		media.class = "Stream/Output/Audio"
`

func TestTheTextListingFindsTheSameBrowser(t *testing.T) {
	playing := playingPrograms(realListing)

	if len(playing) != 1 || playing[0] != "Brave" {
		t.Fatalf("expected only the browser, got %v", playing)
	}
}

// The properties of one node arrive in whatever order PipeWire feels like, so
// the block is finished rather than read in sequence. This is the order that
// would break a reader written the obvious way.
func TestTheClassCanArriveBeforeTheName(t *testing.T) {
	listing := `	id 1, type PipeWire:Interface:Node/3
 		media.class = "Stream/Output/Audio"
 		application.name = "Spotify"
	id 2, type PipeWire:Interface:Node/3
 		application.name = "Brave"
 		media.class = "Stream/Output/Audio"
`

	playing := playingPrograms(listing)

	if len(playing) != 2 {
		t.Fatalf("expected both, got %v", playing)
	}
}

// The last node in a listing has no node after it to end its block.
func TestTheLastNodeInTheListingIsNotLost(t *testing.T) {
	listing := `	id 1, type PipeWire:Interface:Node/3
 		node.name = "Dummy-Driver"
	id 2, type PipeWire:Interface:Node/3
 		application.name = "mpv"
 		media.class = "Stream/Output/Audio"
`

	playing := playingPrograms(listing)

	if len(playing) != 1 || playing[0] != "mpv" {
		t.Fatalf("the last node was dropped: %v", playing)
	}
}

// A name with a space and an equals sign in it, because programs are named by
// people.
func TestAwkwardNamesSurviveTheParser(t *testing.T) {
	listing := `	id 1, type PipeWire:Interface:Node/3
 		application.name = "Firefox — 1 = 2"
 		media.class = "Stream/Output/Audio"
`

	if playing := playingPrograms(listing); len(playing) != 1 ||
		!strings.Contains(playing[0], "Firefox") {
		t.Fatalf("got %v", playing)
	}
}

// Neither reader may panic on a truncated command, which is what a listing
// looks like when the daemon restarts halfway through printing one.
func TestNeitherReaderPanicsOnATruncatedListing(t *testing.T) {
	for cut := 0; cut < len(realListing); cut += 7 {
		playingPrograms(realListing[:cut])
	}

	for cut := 0; cut < len(realDump); cut += 11 {
		playingFromDump([]byte(realDump[:cut]))
	}
}

// Both readers should agree about a machine playing music, since one is the
// other's fallback and the answers reach somebody as the same sentence.
func TestBothReadersAgreeAboutThisMachine(t *testing.T) {
	fromDump, err := playingFromDump([]byte(realDump))
	if err != nil {
		t.Fatal(err)
	}

	fromListing := playingPrograms(realListing)

	if len(fromDump) != len(fromListing) || fromDump[0] != fromListing[0] {
		t.Fatalf("the two readers disagree: %v against %v", fromDump, fromListing)
	}
}
