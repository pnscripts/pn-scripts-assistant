package server

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The pulse says where things stand once, then only what changes: a page
 * that rests on it must be woken by a turn starting and ending, and must not
 * be woken by nothing happening.
 */
func TestThePulseSpeaksOnlyOfChanges(t *testing.T) {
	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	agreedTo(t, root)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(brain.New(db, config.Default(), root, filepath.Join(root, "brain.sqlite"), logger), logger)
	srv.pulseEvery = 10 * time.Millisecond

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	progress.Done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/pulse", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type %q", got)
	}

	events := make(chan string, 16)

	go func() {
		scan := bufio.NewScanner(resp.Body)
		for scan.Scan() {
			if data, ok := strings.CutPrefix(scan.Text(), "data: "); ok {
				events <- data
			}
		}
		close(events)
	}()

	next := func(within time.Duration) (string, bool) {
		select {
		case e, ok := <-events:
			return e, ok
		case <-time.After(within):
			return "", false
		}
	}

	if e, ok := next(2 * time.Second); !ok || !strings.Contains(e, `"busy":false`) {
		t.Fatalf("first pulse %q", e)
	}

	// Twenty looks with nothing happening say nothing.
	if e, ok := next(200 * time.Millisecond); ok {
		t.Fatalf("a pulse with nothing changed: %q", e)
	}

	progress.Begin()
	defer progress.Done()

	if e, ok := next(2 * time.Second); !ok || !strings.Contains(e, `"busy":true`) || !strings.Contains(e, `"kind":"thinking"`) {
		t.Fatalf("a turn starting: %q", e)
	}

	progress.Done()

	if e, ok := next(2 * time.Second); !ok || !strings.Contains(e, `"busy":false`) {
		t.Fatalf("a turn ending: %q", e)
	}

	// Something learned is told at the next count (sixty looks later), as
	// a change in memory alone.
	if _, err := db.AddFact("note", "the pulse counts this", nil); err != nil {
		t.Fatal(err)
	}

	if e, ok := next(3 * time.Second); !ok || !strings.Contains(e, `"memory":1`) || !strings.Contains(e, `"busy":false`) {
		t.Fatalf("something learned: %q", e)
	}
}
