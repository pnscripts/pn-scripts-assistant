package voiceprint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Downloading reports a download in progress, so the page can say so instead of
// starting another.
func TestDownloadingIsTrueOnlyWhileADownloadRuns(t *testing.T) {
	into := machine(t)

	os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)

	archive := releaseArchive(t, RuntimeVersion, []byte("lib"))
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	oldFrom, oldSum := runtimeFrom, runtimeSum
	runtimeFrom, runtimeSum = srv.URL, sumOf(archive)

	t.Cleanup(func() { runtimeFrom, runtimeSum = oldFrom, oldSum })

	if Downloading() {
		t.Fatal("fetching before anything started")
	}

	done := make(chan error, 1)

	go func() { done <- Install(context.Background(), nil) }()

	deadline := time.Now().Add(5 * time.Second)
	for !Downloading() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if !Downloading() {
		t.Fatal("not fetching while the download runs")
	}

	close(release)

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if Downloading() {
		t.Error("still fetching after it finished")
	}
}

/*
 * The model comes from one commit, and only as that commit had it.
 *
 * It was fetched from the repository's main branch with no check at all, so
 * whatever that branch held on the day was what the program loaded, and a
 * server that never stopped sending could fill the disk.
 */
func TestTheModelIsPinnedToOneCommitAndOneHash(t *testing.T) {
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ModelRevision) {
		t.Errorf("ModelRevision %q is not a full commit id", ModelRevision)
	}

	if !strings.Contains(ModelFrom, "/resolve/"+ModelRevision+"/") || strings.Contains(ModelFrom, "/resolve/main/") {
		t.Errorf("ModelFrom %q does not name the pinned commit", ModelFrom)
	}

	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(ModelSHA256) {
		t.Errorf("ModelSHA256 %q is not a SHA-256 in lowercase hex", ModelSHA256)
	}

	if modelFrom != ModelFrom || modelSum != ModelSHA256 {
		t.Error("the model source differs from the published constants outside a test")
	}

	if ModelBytes <= 0 || ModelBytes > mostModelBytes {
		t.Errorf("ModelBytes %d is not within the limit %d", ModelBytes, mostModelBytes)
	}
}

// serveModel points the model download at a local server.
func serveModel(t *testing.T, handler http.HandlerFunc, want string) *atomic.Int32 {
	t.Helper()

	var asked atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	oldFrom, oldSum := modelFrom, modelSum
	modelFrom, modelSum = srv.URL+"/model.onnx", want

	t.Cleanup(func() { modelFrom, modelSum = oldFrom, oldSum })

	return &asked
}

func TestTheModelIsInstalledOnlyWhenItIsThePublishedOne(t *testing.T) {
	published := []byte("the published model")

	t.Run("the published one", func(t *testing.T) {
		into := machine(t)
		archive := releaseArchive(t, RuntimeVersion, []byte("lib"))
		serve(t, archive, sumOf(archive))
		serveModel(t, func(w http.ResponseWriter, r *http.Request) { w.Write(published) }, sumOf(published))

		if err := Install(context.Background(), nil); err != nil {
			t.Fatalf("the published model was refused: %v", err)
		}

		if got := read(t, filepath.Join(into, ModelName)); got != string(published) {
			t.Errorf("installed %q", got)
		}

		if extra := leftovers(t, into, ModelName, RuntimeName, RuntimeStamp); len(extra) > 0 {
			t.Errorf("left behind: %v", extra)
		}
	})

	t.Run("a changed one", func(t *testing.T) {
		into := machine(t)
		archive := releaseArchive(t, RuntimeVersion, []byte("lib"))
		runtimeAsked := serve(t, archive, sumOf(archive))
		serveModel(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("something else")) }, sumOf(published))

		err := Install(context.Background(), nil)
		if !errors.Is(err, ErrModelTampered) {
			t.Fatalf("a changed model gave %v, want ErrModelTampered", err)
		}

		if extra := leftovers(t, into); len(extra) > 0 {
			t.Errorf("a refused model left %v", extra)
		}

		if runtimeAsked.Load() != 0 {
			t.Error("the runtime was fetched after the model was refused")
		}
	})

	t.Run("an endless one", func(t *testing.T) {
		into := machine(t)
		serveModel(t, func(w http.ResponseWriter, r *http.Request) {
			chunk := make([]byte, 1<<20)

			for i := 0; i <= mostModelBytes>>20+1; i++ {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		}, sumOf(published))

		if err := Install(context.Background(), nil); !errors.Is(err, ErrModelTampered) {
			t.Fatalf("an oversized model gave %v", err)
		}

		if extra := leftovers(t, into); len(extra) > 0 {
			t.Errorf("an oversized model left %v", extra)
		}
	})
}
