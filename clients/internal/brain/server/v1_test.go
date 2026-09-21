package server

import (
	"strconv"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/protocol"
)

/*
 * The versioned API says what it speaks, and what happened after a number.
 *
 * Both are what a program other than this page needs before it can do
 * anything: one to know whether it can talk to this at all, the other to
 * catch up on what it missed while it was not connected.
 */
func TestTheVersionedAPISaysWhatItSpeaksAndWhatHappened(t *testing.T) {
	ts, _, b := newServer(t)

	var version struct {
		Product  string `json:"product"`
		Protocol int    `json:"protocol"`
		Platform string `json:"platform"`
		Activity int64  `json:"activity"`
	}

	getJSON(t, ts.URL+"/api/v1/version", &version)

	if version.Protocol != protocol.Version || version.Product == "" || !strings.Contains(version.Platform, "/") {
		t.Fatalf("it did not say what it is: %+v", version)
	}

	first := b.Happens.Say(protocol.New(protocol.TaskStarted, "Catch falling stars").About(1, 0))
	second := b.Happens.Say(protocol.New(protocol.StepStarted, "write the game").About(1, 7))

	var caught struct {
		Protocol int                 `json:"protocol"`
		Events   []protocol.Envelope `json:"events"`
		Latest   int64               `json:"latest"`
	}

	getJSON(t, ts.URL+"/api/v1/activity?since=0", &caught)

	if len(caught.Events) != 2 || caught.Events[0].Seq != first.Seq || caught.Latest != second.Seq {
		t.Fatalf("catching up from the beginning gave %+v", caught)
	}

	if caught.Events[1].Said != "write the game" || caught.Events[1].Task != 1 || caught.Events[1].Step != 7 {
		t.Errorf("an event lost what it was about: %+v", caught.Events[1])
	}

	// And from a number, only what came after it.
	getJSON(t, ts.URL+"/api/v1/activity?since="+strconv.FormatInt(first.Seq, 10), &caught)

	if len(caught.Events) != 1 || caught.Events[0].Seq != second.Seq {
		t.Errorf("catching up from %d gave %+v", first.Seq, caught.Events)
	}
}
