package speech

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * pw-dump does not always print one document.
 *
 * This is a regression test for a fault that reached somebody: the microphone
 * list answered 500 with "invalid character '[' after top-level value", so the
 * page could not name a single input. Which looks exactly like a broken
 * microphone — and "it cannot hear me" already has five causes without adding
 * a JSON parser meeting a second array to the list.
 *
 * It happens under churn: a stream starting or stopping while pw-dump runs,
 * which on this machine is every time the brain speaks.
 */

const twoArrays = `[
{"id":40,"info":{"state":"idle","props":{"media.class":"Audio/Source","node.name":"mic"}}}
]
[
{"id":41,"info":{"state":"running","props":{"media.class":"Stream/Output/Audio","application.name":"Brave"}}}
]`

func TestASecondArrayIsReadRatherThanRefused(t *testing.T) {
	found, err := dumpObjects([]byte(twoArrays))
	if err != nil {
		t.Fatalf("a listing printed as two arrays was refused: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("read %d objects across the two arrays, want 2", len(found))
	}
}

// And the reader that matters most: this is what returned the 500.
func TestTheMicrophoneListSurvivesASplitListing(t *testing.T) {
	found, err := dumpObjects([]byte(twoArrays))
	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, one := range found {
		var object struct {
			Info struct {
				Props map[string]any `json:"props"`
			} `json:"info"`
		}

		if err := json.Unmarshal(one, &object); err != nil {
			continue
		}

		if object.Info.Props["media.class"] == "Audio/Source" {
			names = append(names, object.Info.Props["node.name"].(string))
		}
	}

	if len(names) != 1 || names[0] != "mic" {
		t.Fatalf("the microphone was lost in a split listing: %v", names)
	}
}

// The other reader too, since it takes the same output.
func TestWhatIsPlayingIsFoundInASplitListing(t *testing.T) {
	playing, err := playingFromDump([]byte(twoArrays))
	if err != nil {
		t.Fatalf("a split listing was refused: %v", err)
	}

	if len(playing) != 1 || playing[0] != "Brave" {
		t.Fatalf("the second array was dropped: %v", playing)
	}
}

/*
 * An empty listing is an answer, not a failure.
 *
 * A machine playing nothing is the normal state of a quiet room, and treating
 * it as an error would have the brain report a broken sound card every time
 * nobody was playing music.
 */
func TestAnEmptyListingIsNotAnError(t *testing.T) {
	found, err := dumpObjects([]byte("[]"))
	if err != nil {
		t.Fatalf("an empty listing was called an error: %v", err)
	}

	if len(found) != 0 {
		t.Fatalf("invented %d objects", len(found))
	}
}

// A second array cut off halfway — the daemon restarting mid-print — keeps
// what was already read rather than losing the whole listing.
func TestATruncatedSecondArrayKeepsTheFirst(t *testing.T) {
	cut := twoArrays[:len(twoArrays)-40]

	found, err := dumpObjects([]byte(cut))
	if err != nil {
		t.Fatalf("the whole listing was thrown away: %v", err)
	}

	if len(found) != 1 {
		t.Fatalf("kept %d objects from the readable part, want 1", len(found))
	}
}

// Nothing readable at all is still an error, so a caller can fall back rather
// than report an empty machine.
func TestNothingReadableIsStillAnError(t *testing.T) {
	for _, raw := range []string{"", "not json", "pw-dump: command not found\n"} {
		if _, err := dumpObjects([]byte(raw)); err == nil {
			t.Errorf("%q was accepted as a listing", raw)
		}
	}
}

// One malformed object must not cost somebody the rest of their microphones.
func TestOneBadObjectDoesNotLoseTheOthers(t *testing.T) {
	listing := `[
	{"id":1,"info":{"props":{"media.class":"Stream/Output/Audio","application.name":"Brave"}},"state":1},
	{"id":2,"info":{"state":"running","props":{"media.class":"Stream/Output/Audio","application.name":"Spotify"}}}
	]`

	// The first object's info.state is a number where a string is expected,
	// which is what a version difference looks like.
	listing = strings.Replace(listing, `"state":1`, `"state":"running"`, 1)

	playing, err := playingFromDump([]byte(listing))
	if err != nil {
		t.Fatal(err)
	}

	if len(playing) != 1 || playing[0] != "Spotify" {
		t.Fatalf("got %v", playing)
	}
}
