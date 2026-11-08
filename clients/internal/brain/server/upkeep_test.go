package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"pn-scripts-assistant/internal/brain/latest"
	"pn-scripts-assistant/internal/brain/release"
)

// releasedAs makes this test binary a released copy of a version.
func releasedAs(t *testing.T, version string) {
	t.Helper()

	was := release.Version
	release.Version = version

	t.Cleanup(func() { release.Version = was })
}

// newerOut serves a newest release the way GitHub's API does, counting how
// often it is asked.
func newerOut(t *testing.T, version string) *atomic.Int32 {
	t.Helper()

	var asked atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"https://github.com/pnscripts/pn-scripts-assistant/releases/tag/v%s","assets":[]}`,
			version, version)
	}))

	t.Cleanup(srv.Close)
	t.Setenv(latest.TestEnv, srv.URL)

	return &asked
}

func TestAnOlderReleasedCopySeesTheNewVersion(t *testing.T) {
	releasedAs(t, "0.11.0")
	asked := newerOut(t, "0.12.0")

	_, _, b := newServer(t)
	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	item := s.programUpkeep(context.Background(), false)

	if !item.Newer || item.Have != "0.11.0" || item.Latest != "0.12.0" {
		t.Fatalf("the program's row: %+v", item)
	}

	if !strings.HasSuffix(item.Page, "/v0.12.0") || item.Channel == "" {
		t.Fatalf("no release page or channel: %+v", item)
	}

	// Asked once a day, however often the page looks.
	for i := 0; i < 5; i++ {
		s.programUpkeep(context.Background(), false)
	}

	if n := asked.Load(); n != 1 {
		t.Fatalf("GitHub was asked %d times, want once", n)
	}

	// "Check again" asks at once.
	s.programUpkeep(context.Background(), true)

	if n := asked.Load(); n != 2 {
		t.Fatalf("Check again did not ask (%d)", n)
	}
}

func TestTheSameVersionIsCurrent(t *testing.T) {
	releasedAs(t, "0.12.0")
	newerOut(t, "0.12.0")

	_, _, b := newServer(t)
	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if item := s.programUpkeep(context.Background(), false); item.Newer || item.Why != "" {
		t.Fatalf("an up-to-date copy was told otherwise: %+v", item)
	}
}

func TestWithTheNoticeOffNothingIsAskedUnlessAsked(t *testing.T) {
	releasedAs(t, "0.11.0")
	asked := newerOut(t, "0.12.0")

	_, _, b := newServer(t)
	b.Cfg.UpdateNotice = false

	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	item := s.programUpkeep(context.Background(), false)

	if asked.Load() != 0 || item.WhyKey != "upd.noticeOff" || item.Newer {
		t.Fatalf("with the notice off: asked %d, %+v", asked.Load(), item)
	}

	if item := s.programUpkeep(context.Background(), true); !item.Newer || asked.Load() != 1 {
		t.Fatalf("Check again with the notice off: asked %d, %+v", asked.Load(), item)
	}
}

func TestOfflineSaysSoAndIsNotKept(t *testing.T) {
	releasedAs(t, "0.11.0")
	t.Setenv(latest.TestEnv, "http://127.0.0.1:9/latest")

	_, _, b := newServer(t)
	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	item := s.programUpkeep(context.Background(), false)

	if item.WhyKey != "upd.offline" || item.Newer {
		t.Fatalf("offline: %+v", item)
	}

	// The network back: noticed at the next look, not a day later.
	newerOut(t, "0.12.0")

	if item := s.programUpkeep(context.Background(), false); !item.Newer {
		t.Fatalf("the network came back and was not noticed: %+v", item)
	}
}

func TestTheNoticeSwitchIsKept(t *testing.T) {
	ts, _, b := newServer(t)

	resp, err := http.Post(ts.URL+"/api/upkeep/notice", "application/json", strings.NewReader(`{"on":false}`))
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()

	if resp.StatusCode != 200 || b.Cfg.UpdateNotice {
		t.Fatalf("switching it off: %d, still %v", resp.StatusCode, b.Cfg.UpdateNotice)
	}
}

func TestACopyNothingInstalledIsSentToTheReleasePage(t *testing.T) {
	releasedAs(t, "0.11.0")
	newerOut(t, "0.12.0")

	ts, _, _ := newServer(t)

	// This test binary sits in a build folder: a loose copy, which nothing
	// may replace by itself.
	resp, err := http.Post(ts.URL+"/api/upkeep/self", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	var out latest.Outcome

	json.NewDecoder(resp.Body).Decode(&out)

	if resp.StatusCode != 200 || out.Started || !strings.HasSuffix(out.Open, "/v0.12.0") {
		t.Fatalf("%d %+v", resp.StatusCode, out)
	}
}
