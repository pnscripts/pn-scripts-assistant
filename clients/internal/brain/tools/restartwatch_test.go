package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/machinecare"
)

type restartWatcher struct {
	conversation int64
	asked        time.Duration
	watch        machinecare.Watch
	stopped      bool
}

func (r *restartWatcher) WatchForRestart(conversation int64, d time.Duration) (machinecare.Watch, bool, error) {
	r.conversation, r.asked = conversation, d

	w, err := machinecare.NewWatch(time.Now(), d)
	if err != nil {
		return w, false, err
	}

	w.Look(machinecare.Judge(machinecare.Inputs{Installing: []string{"apt"}}), time.Now())

	return w, false, nil
}

func (r *restartWatcher) StopRestartWatch() (machinecare.Watch, bool) {
	was := !r.stopped
	r.stopped = true

	return machinecare.Watch{State: machinecare.Stopped}, was
}

// The watch is started for the conversation the call came from, for the
// hours asked, and the answer says until when and what is in the way.
func TestWatchForRestartStartsAWatchForThisConversation(t *testing.T) {
	r := &restartWatcher{}
	tool := WatchForRestart{Care: r}

	out, err := tool.Execute(InConversation(context.Background(), 42), json.RawMessage(`{"hours":2}`))
	if err != nil {
		t.Fatal(err)
	}

	if r.conversation != 42 || r.asked != 2*time.Hour {
		t.Fatalf("asked for %d, %s", r.conversation, r.asked)
	}

	if !strings.HasPrefix(out, "Watching: I look every 2 minutes until ") ||
		!strings.Contains(out, "In the way now: apt is installing or updating software") {
		t.Errorf("%q", out)
	}

	// No arguments at all is the default watch.
	if _, err := tool.Execute(context.Background(), nil); err != nil || r.asked != 0 || r.conversation != 0 {
		t.Errorf("default: %v %s %d", err, r.asked, r.conversation)
	}

	for _, bad := range []string{`{"hours":13}`, `{"hours":-1}`, `{"hours":0.1}`} {
		if _, err := tool.Execute(context.Background(), json.RawMessage(bad)); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}

	if out, _ := tool.Execute(context.Background(), json.RawMessage(`{"stop":true}`)); out != "Stopped watching for a safe restart." {
		t.Errorf("%q", out)
	}

	if out, _ := tool.Execute(context.Background(), json.RawMessage(`{"stop":true}`)); out != "No restart was being watched for." {
		t.Errorf("%q", out)
	}

	if tool.Summarize(json.RawMessage(`{"stop":true}`)) == tool.Summarize(nil) {
		t.Error("stopping and starting read the same")
	}
}
