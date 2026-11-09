package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/progress"
)

/*
 * The runtime refresh at start leaves an answer's step on screen.
 *
 * It said what it was doing with SetBackground and ended with Done, which
 * write over and clear whatever is current: a refresh finishing while a
 * question was being answered erased "Thinking", and the page showed an idle
 * brain halfway through the answer.
 */
func TestTheRuntimeRefreshLeavesTheAnswerOnScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := config.Default()
	cfg.LookOnline = true

	s := &Server{
		brain: &brain.Brain{Cfg: cfg, Root: t.TempDir()},
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	old := refreshRuntime
	t.Cleanup(func() { refreshRuntime = old })

	refreshRuntime = func(ctx context.Context, say func(string)) (bool, error) {
		say("Updating what tells voices apart")

		return true, nil
	}

	progress.Begin()
	progress.Set("thinking", "Thinking")
	t.Cleanup(progress.Done)

	s.refreshVoiceprint(context.Background())

	if step := progress.Now(); !step.Busy || step.Kind != "thinking" || step.Note != "Thinking" {
		t.Errorf("the refresh wrote over or cleared the answer's step: %+v", step)
	}
}

/*
 * The Install button starts one download, and leaves an answer alone.
 *
 * Pressed twice, or pressed while the refresh at start was already fetching,
 * it started a second download into the same files. Now a second press says
 * a download is under way, and the one download ends without clearing the
 * step of a question answered meanwhile.
 */
func TestTheInstallButtonStartsOneDownload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ts, _, _ := newServer(t)

	release := make(chan struct{})

	var calls atomic.Int32

	finished := make(chan struct{})

	old := installVoiceprint
	t.Cleanup(func() { installVoiceprint = old })

	installVoiceprint = func(ctx context.Context, say func(string)) error {
		calls.Add(1)
		say("Downloading the model that tells voices apart")
		<-release

		defer close(finished)

		return nil
	}

	status, first := postBody(t, ts.URL+"/api/voiceprint/install", `{}`)
	if status != http.StatusOK || first["started"] != true {
		t.Fatalf("the first press did not start the download: %d %v", status, first)
	}

	status, second := postBody(t, ts.URL+"/api/voiceprint/install", `{}`)
	if status != http.StatusOK || second["started"] == true || second["downloading"] != true {
		t.Errorf("a second press while it downloads: %d %v; want it told a download is running", status, second)
	}

	// A question starts while it downloads.
	progress.Begin()
	progress.Set("thinking", "Thinking")
	t.Cleanup(progress.Done)

	close(release)

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the download never finished")
	}

	// The goroutine's own ending runs after the fake returns.
	deadline := time.Now().Add(2 * time.Second)
	for installing.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("%d downloads were started, want one", n)
	}

	if step := progress.Now(); !step.Busy || step.Kind != "thinking" || step.Note != "Thinking" {
		t.Errorf("the download wrote over or cleared the answer's step: %+v", step)
	}

	// Done, a press starts a new one.
	release = make(chan struct{})
	finished = make(chan struct{})

	close(release)

	if status, again := postBody(t, ts.URL+"/api/voiceprint/install", `{}`); status != http.StatusOK || again["started"] != true {
		t.Errorf("after the download a press did not start another: %d %v", status, again)
	}

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the second download never finished")
	}
}
