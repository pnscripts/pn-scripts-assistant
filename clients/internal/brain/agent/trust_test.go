package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/cite"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Every call is told to Did with what led to it, and a file read during a
 * conversation that keeps a ledger is numbered where the model reads it —
 * while the step itself keeps the file's own text.
 */
func TestEveryCallIsRecordedAndReadingIsNumbered(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.txt")
	target := filepath.Join(dir, "out.txt")

	if err := os.WriteFile(notes, []byte("The meeting is on Tuesday."), 0o644); err != nil {
		t.Fatal(err)
	}

	loop, db := newLoop(t, tools.ReadFile{}, tools.WriteFile{})

	var deeds []Deed

	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "read_file",
			Arguments: json.RawMessage(`{"path":` + quote(notes) + `}`),
		}}},
		{ToolCalls: []llm.ToolCall{{
			ID: "c2", Name: "write_file",
			Arguments: json.RawMessage(`{"path":` + quote(target) + `,"content":"x"}`),
		}}},
	}}

	conv, _ := db.NewConversation("t")

	ledger := cite.NewLedger()
	ctx := cite.With(context.Background(), ledger)

	res, err := loop.Run(ctx, conv, model, []llm.Message{{Role: llm.RoleUser, Content: "when is the meeting?"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(deeds) != 2 {
		t.Fatalf("%d calls recorded, expected 2: %+v", len(deeds), deeds)
	}

	if deeds[0].Outcome != DeedDone || deeds[0].Step.Tool != "read_file" {
		t.Errorf("the read was recorded as %+v", deeds[0])
	}

	if deeds[0].Request != "when is the meeting?" {
		t.Errorf("the request behind it reads %q", deeds[0].Request)
	}

	if !strings.Contains(string(deeds[0].Args), "notes.txt") {
		t.Errorf("the call's arguments were not kept: %s", deeds[0].Args)
	}

	if deeds[1].Outcome != DeedWaiting || deeds[1].Step.Tool != "write_file" {
		t.Errorf("the write was recorded as %+v", deeds[1])
	}

	// The model read the file with its number on it.
	labelled := false

	for _, m := range model.seen[len(model.seen)-1].Messages {
		if m.Role == llm.RoleTool && strings.HasPrefix(m.Content, "Source [1].") {
			labelled = true
		}
	}

	if !labelled {
		t.Error("the file was not numbered where the model reads it")
	}

	// And the step keeps the file's own words.
	if len(res.Steps) == 0 || strings.Contains(res.Steps[0].Result, "Source [") {
		t.Errorf("the step's result was changed: %+v", res.Steps)
	}

	if got := ledger.Sources(); len(got) != 1 || got[0].Where != notes {
		t.Errorf("ledger %+v", got)
	}
}
