package speech

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Turning the music down while it talks.
 *
 * The whole risk in this is asymmetric: failing to turn the music down means
 * an answer is hard to hear once, and leaving it down means somebody's
 * speakers are quiet for good with no idea why. That second one happened here
 * over and over, each time by a different road, so most of what is tested here
 * is that no road leads there any more.
 */

func TestSpeakingIsCountedSoOverlappingSentencesDoNotUnbalance(t *testing.T) {
	inRoom(t)
	reset()

	first := duckOthers(nothing())
	second := duckOthers(nothing())

	first()

	duckMu.Lock()
	still := speaking
	duckMu.Unlock()

	if still != 1 {
		t.Fatalf("one sentence finishing left the count at %d, want 1", still)
	}

	second()

	duckMu.Lock()
	none := speaking
	duckMu.Unlock()

	if none != 0 {
		t.Fatalf("both finished but the count is %d", none)
	}
}

/*
 * The level is held down between the sentences of one answer, and a sentence
 * starting during the hold cancels the restore rather than letting it fire
 * mid-answer.
 */
func TestTheLevelIsHeldBetweenSentences(t *testing.T) {
	inRoom(t)
	reset()

	duckOthers(nothing())()

	duckMu.Lock()
	pending := restorer != nil
	duckMu.Unlock()

	if !pending {
		t.Fatal("the level was restored immediately rather than held")
	}

	release := duckOthers(nothing())

	duckMu.Lock()
	cancelled := restorer == nil
	duckMu.Unlock()

	if !cancelled {
		t.Fatal("the next sentence did not cancel the pending restore")
	}

	release()
}

// What goes down is a fifth of the stream's own level, and what comes back up
// is its own level: the same stream, found by serial.
func TestTheRoomGoesDownToAFifthAndBackToItsOwnLevel(t *testing.T) {
	room := inRoom(t, playing("Brave", 51, "900", 0.8, 0.8))
	reset()

	release := duckOthers(nothing())

	if got := room.gain["900"]; len(got) != 2 || !near(got[0], 0.16) || !near(got[1], 0.16) {
		t.Fatalf("Brave was taken to %v, want a fifth of 0.8", got)
	}

	release()
	PutTheVolumeBack()

	if got := room.gain["900"]; len(got) != 2 || !near(got[0], 0.8) {
		t.Fatalf("Brave came back at %v, want its own 0.8", got)
	}
}

/*
 * Back to the level its owner has now.
 *
 * Somebody who turns their film up while the brain is talking has chosen a
 * new level; putting back the one from before they touched it would undo that.
 */
func TestItComesBackToTheLevelChosenMeanwhile(t *testing.T) {
	room := inRoom(t, playing("Brave", 51, "900", 0.5, 0.5))
	reset()

	release := duckOthers(nothing())

	room.streams[0].Volumes = []float64{0.9, 0.9}

	release()
	PutTheVolumeBack()

	if got := room.gain["900"]; !near(got[0], 0.9) {
		t.Fatalf("came back at %v, want the 0.9 chosen while it was down", got)
	}
}

// A paused stream is not turned down: it is making no noise, and turning it
// down would mean it changing level by itself when it plays again.
func TestAPausedStreamIsLeftAlone(t *testing.T) {
	paused := playing("mpv", 60, "901", 1, 1)
	paused[0].Running = false

	room := inRoom(t, paused)
	reset()

	duckOthers(nothing())()
	PutTheVolumeBack()

	if len(room.gain) != 0 {
		t.Fatalf("a paused stream was touched: %v", room.gain)
	}
}

// A stream that ended while it was down needs nothing, and nothing is kept
// about it: its gain went with it.
func TestAStreamThatEndedLeavesNothingBehind(t *testing.T) {
	room := inRoom(t, playing("Brave", 51, "900", 1, 1))
	reset()

	release := duckOthers(nothing())

	room.streams = nil
	room.gain = map[string][]float64{}

	release()
	PutTheVolumeBack()

	duckMu.Lock()
	defer duckMu.Unlock()

	if lowered != nil || len(room.gain) != 0 {
		t.Fatalf("something was kept or set for a stream that is gone: %v %v", lowered, room.gain)
	}
}

// Turning the feature off puts back anything currently down, at once.
func TestSwitchingItOffPutsTheLevelBackAtOnce(t *testing.T) {
	room := inRoom(t, playing("Brave", 51, "900", 1, 1))
	reset()

	duckOthers(nothing())

	DuckOthersWhileTalking(false)
	defer DuckOthersWhileTalking(true)

	duckMu.Lock()
	left, count := len(lowered), speaking
	duckMu.Unlock()

	if left != 0 || count != 0 {
		t.Fatalf("%d streams left down and %d sentences still counted", left, count)
	}

	if got := room.gain["900"]; !near(got[0], 1) {
		t.Errorf("Brave was left at %v, want 1", got)
	}
}

/*
 * Lowering can take longer than the hold before putting it back.
 *
 * When it did, the restore ran first and found nothing, then the lowering
 * finished — and the music stayed down for as long as the brain went on
 * thinking. So lowering that finishes after the last sentence puts it back
 * itself.
 */
func TestLoweringThatFinishesAfterTheReleaseStillPutsItBack(t *testing.T) {
	room := inRoom(t, playing("Brave", 51, "900", 1, 1))
	reset()

	// Nothing is speaking by the time the lowering gets to the end.
	lower(nothing())

	duckMu.Lock()
	defer duckMu.Unlock()

	if lowered != nil {
		t.Fatal("the lowering was recorded with nothing left speaking")
	}

	if got := room.gain["900"]; !near(got[0], 1) {
		t.Errorf("Brave was left at %v, want 1", got)
	}
}

/*
 * Its own voice is never turned down.
 *
 * It was, for weeks: espeak calls its stream "eSpeak", which was not on the
 * list of names the brain knew as its own, so it lowered its own voice with
 * every sentence and the setting was saved. Now the question is asked of the
 * process tree first, which does not depend on what an engine calls itself.
 */
func TestItNeverTurnsItsOwnVoiceDown(t *testing.T) {
	for _, name := range []string{
		"pn-brain.echo-cancel.playback", "pw-play", "piper",
		"speech-dispatcher-espeak-ng", "PN-Brain", "eSpeak",
	} {
		if !ours(name) {
			t.Errorf("%s would have been turned down", name)
		}
	}

	for _, name := range []string{"Brave", "Spotify", "mpv", "Firefox"} {
		if ours(name) {
			t.Errorf("%s was mistaken for our own voice", name)
		}
	}
}

func TestAnythingThisProgramStartedIsItsOwn(t *testing.T) {
	me := os.Getpid()
	tree := map[int]int{5001: me, 5002: 5001, 6001: 1}

	was := parentOf
	parentOf = func(pid int) int { return tree[pid] }

	t.Cleanup(func() { parentOf = was })

	if !startedByMe(5001) || !startedByMe(5002) {
		t.Error("a child or grandchild of this program was not recognised as its own")
	}

	if startedByMe(6001) || startedByMe(0) {
		t.Error("somebody else's process was taken for this program's")
	}
}

/*
 * The one thing that must never come back.
 *
 * WirePlumber saves channelVolumes, volume and mute against the program. The
 * gain handed to a stream carries softVolumes and nothing else, so turning
 * the room down cannot be saved.
 */
func TestTurningDownSetsNothingThatIsSaved(t *testing.T) {
	arg := gainArgument([]float64{0.2, 0.2})

	if arg != "{ softVolumes: [ 0.2000, 0.2000 ] }" {
		t.Fatalf("the gain was written as %q", arg)
	}

	for _, saved := range []string{"channelVolumes", "volume:", "mute"} {
		if strings.Contains(arg, saved) {
			t.Fatalf("the gain carries %s, which WirePlumber saves", saved)
		}
	}
}

/*
 * And nothing in this package sets a saved level, except the repair.
 *
 * The old way was one line: wpctl set-volume. A test that reads the source is
 * blunt, and it is the only kind that stops that line being written again in
 * some new file by somebody who has not read this one.
 */
func TestOnlyTheRepairSetsASavedLevel(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "savedlevels.go" {
			continue
		}

		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}

		source := string(raw)

		for _, forbidden := range []string{`"set-volume"`, "channelVolumes:", `"set-mute"`} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s uses %s, which sets a level WirePlumber saves against another program", name, forbidden)
			}
		}
	}
}

/*
 * pw-dump's streams are read with their serial, their process and both
 * levels.
 *
 * The process is on the client, not the stream — the shape below is what
 * pw-dump printed for an aplay stream on this machine — and reading it off the
 * stream found nothing for any stream at all.
 */
func TestStreamsAreReadWithTheirLevelsAndProcess(t *testing.T) {
	raw := []byte(`[
	  {"id": 116, "type": "PipeWire:Interface:Client",
	   "info": {"props": {"application.name": "Brave", "application.process.id": 3711}}},
	  {"id": 93, "type": "PipeWire:Interface:Node",
	   "info": {"state": "running",
	    "props": {"media.class": "Stream/Output/Audio", "application.name": "Brave",
	              "object.serial": 4117, "client.id": 116},
	    "params": {"Props": [
	      {"volume": 1.0, "mute": false, "channelVolumes": [0.8, 0.8], "softVolumes": [0.16, 0.16]},
	      {"params": []}
	    ]}}},
	  {"id": 94, "type": "PipeWire:Interface:Node",
	   "info": {"state": "running",
	    "props": {"media.class": "Audio/Sink", "node.name": "speakers"}}}
	]`)

	got := readStreams(raw)

	if len(got) != 1 {
		t.Fatalf("read %d streams, want only the output stream", len(got))
	}

	s := got[0]

	if s.ID != 93 || s.Serial != "4117" || s.Process != 3711 || s.Name != "Brave" || !s.Running {
		t.Errorf("read as %+v", s.stream)
	}

	if len(s.Volumes) != 2 || s.Volumes[0] != 0.8 || s.Gain[0] != 0.16 {
		t.Errorf("levels read as %v and %v", s.Volumes, s.Gain)
	}

	if !leftAtAFifth(s.stream) {
		t.Error("a stream at a fifth of its own level was not recognised as left down")
	}
}

// And a stream whose process is this program is the brain's own, whatever it
// is called.
func TestAStreamThisProgramOpenedIsItsOwn(t *testing.T) {
	raw := []byte(fmt.Sprintf(`[
	  {"id": 7, "type": "PipeWire:Interface:Client",
	   "info": {"props": {"application.process.id": %d}}},
	  {"id": 8, "type": "PipeWire:Interface:Node",
	   "info": {"state": "running",
	    "props": {"media.class": "Stream/Output/Audio", "application.name": "Some Engine",
	              "object.serial": 12, "client.id": 7}}}
	]`, os.Getpid()))

	got := readStreams(raw)

	if len(got) != 1 || !got[0].mine {
		t.Fatalf("a stream this program opened was not taken for its own: %+v", got)
	}
}

func nothing() context.Context { return context.Background() }

func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

func reset() {
	PutTheVolumeBack()

	duckMu.Lock()
	defer duckMu.Unlock()

	lowered = nil
	speaking = 0
}

// fakeRoom is what a test says this machine is playing, and what was set.
type fakeRoom struct {
	streams []stream
	gain    map[string][]float64 // by serial
}

func playing(name string, id int, serial string, levels ...float64) []stream {
	return []stream{{ID: id, Serial: serial, Name: name, Volumes: levels, Gain: levels, Running: true}}
}

/*
 * inRoom says what this machine is playing, for the length of one test.
 *
 * Without it these tests read — and set — the real audio graph, and whether
 * "was anything left turned down" held depended on whether a browser happened
 * to be playing while the suite ran.
 */
func inRoom(t *testing.T, streams ...[]stream) *fakeRoom {
	t.Helper()

	room := &fakeRoom{gain: map[string][]float64{}}

	for _, s := range streams {
		room.streams = append(room.streams, s...)
	}

	wasStreams, wasPut, wasControl := streamsNow, putGain, haveGainControl

	// A room supplied by the test has the tools to turn it down, whatever
	// machine the suite runs on.
	haveGainControl = func() bool { return true }
	streamsNow = func(context.Context) []stream { return room.streams }
	putGain = func(_ context.Context, s stream, levels []float64) bool {
		room.gain[s.Serial] = append([]float64(nil), levels...)

		return true
	}

	t.Cleanup(func() { streamsNow, putGain, haveGainControl = wasStreams, wasPut, wasControl })

	return room
}
