package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

// An envelope survives the journey: what comes out the other side is what
// went in, including the part a reader that does not know the kind keeps.
func TestAnEnvelopeSurvivesBeingSent(t *testing.T) {
	sent := New(ToolOutput, "godot check: passed").About(12, 31).On("dev_ws1").
		With(map[string]any{"stream": "stdout", "lines": 3})
	sent.Seq = 1042

	raw, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}

	var back Envelope

	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}

	if back.Type != ToolOutput || back.Task != 12 || back.Step != 31 || back.Device != "dev_ws1" ||
		back.Seq != 1042 || back.Said != "godot check: passed" || back.V != Version {
		t.Fatalf("it came back different: %+v", back)
	}

	var data struct {
		Stream string `json:"stream"`
		Lines  int    `json:"lines"`
	}

	if err := json.Unmarshal(back.Data, &data); err != nil || data.Stream != "stdout" || data.Lines != 3 {
		t.Errorf("what it carried did not survive: %v %+v", err, data)
	}

	if !strings.HasPrefix(back.ID, "evt_") {
		t.Errorf("an event's id does not say what it is: %q", back.ID)
	}
}

/*
 * A message this program cannot act on is refused, and says why.
 *
 * The two reasons are different and both are worth a sentence: one says the
 * other end is newer than this, which somebody can fix by updating; the other
 * says the kind is unknown, which is a bug rather than a version.
 */
func TestAMessageItCannotActOnIsRefused(t *testing.T) {
	newer := New(TaskStarted, "").About(1, 0)
	newer.V = Version + 1

	if err := newer.Check(); err == nil || !strings.Contains(err.Error(), "update this program") {
		t.Errorf("a newer protocol was accepted, or said nothing useful: %v", err)
	}

	unknown := New(Kind("TASK_TELEPORTED"), "")
	if err := unknown.Check(); err == nil || !strings.Contains(err.Error(), "TASK_TELEPORTED") {
		t.Errorf("an unknown kind was accepted: %v", err)
	}

	empty := Envelope{V: Version}
	if err := empty.Check(); err == nil {
		t.Error("an envelope saying nothing about what happened was accepted")
	}

	if err := New(TaskStarted, "started").Check(); err != nil {
		t.Errorf("a good envelope was refused: %v", err)
	}
}

// Data that cannot be written as JSON costs the detail, never the event: it
// happened whether or not it can be described.
func TestAnEventSurvivesDetailThatCannotBeWritten(t *testing.T) {
	e := New(ProgressUpdate, "still going").With(map[string]any{"how": func() {}})

	if e.Type != ProgressUpdate || e.Said != "still going" {
		t.Errorf("the event was lost with its detail: %+v", e)
	}

	if len(e.Data) != 0 {
		t.Errorf("unwritable detail was kept anyway: %s", e.Data)
	}
}
