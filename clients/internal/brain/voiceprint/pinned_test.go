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
 * A model, when one is offered, comes from one commit and only as that
 * commit had it.
 *
 * It was fetched from the repository's main branch with no check at all, so
 * whatever that branch held on the day was what the program loaded, and a
 * server that never stopped sending could fill the disk. Since 1.0.1 no model
 * is offered (both empty); a release that offers one again must pin it.
 */
func TestTheModelIsPinnedToOneCommitAndOneHash(t *testing.T) {
	if !Offered() {
		if modelFrom != "" || modelSum != "" {
			t.Errorf("half a model is named: from %q, sum %q", modelFrom, modelSum)
		}

		return
	}

	if !regexp.MustCompile(`/resolve/[0-9a-f]{40}/`).MatchString(modelFrom) || strings.Contains(modelFrom, "/resolve/main/") {
		t.Errorf("the model source %q does not name one commit", modelFrom)
	}

	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(modelSum) {
		t.Errorf("the model digest %q is not a SHA-256 in lowercase hex", modelSum)
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

/*
 * A model fetched by 0.13 or earlier is checked once before it is trusted.
 *
 * Those copies took the model from the repository's main branch with no check
 * at all. Pinning the download does nothing for a file already on the disk:
 * the next time the feature is turned on, Install saw a file with the right
 * name and fetched only the runtime, so an unchecked model went on being
 * loaded for good. Now a model already here is hashed before it counts, and
 * one that is not the published file is replaced by one that is.
 */
func TestAModelFromAnOlderReleaseIsCheckedBeforeItIsTrusted(t *testing.T) {
	published := []byte("the published model")

	current := func(t *testing.T, into string) {
		os.WriteFile(filepath.Join(into, RuntimeName), []byte("lib"), 0o755)
		os.WriteFile(filepath.Join(into, RuntimeStamp), []byte(RuntimeVersion+"\n"), 0o644)
	}

	t.Run("the published one is kept and not fetched again", func(t *testing.T) {
		into := machine(t)
		current(t, into)
		os.WriteFile(filepath.Join(into, ModelName), published, 0o644)

		asked := serveModel(t, func(w http.ResponseWriter, r *http.Request) { w.Write(published) }, sumOf(published))

		if !modelVerifiedIn(into) {
			t.Error("the published model already here does not count as checked")
		}

		if err := Install(context.Background(), nil); err != nil {
			t.Fatal(err)
		}

		if n := asked.Load(); n != 0 {
			t.Errorf("the model was fetched %d times although it was the published one", n)
		}
	})

	t.Run("one that is not the published file is replaced", func(t *testing.T) {
		into := machine(t)
		current(t, into)
		os.WriteFile(filepath.Join(into, ModelName), []byte("whatever main held in 0.13"), 0o644)

		asked := serveModel(t, func(w http.ResponseWriter, r *http.Request) { w.Write(published) }, sumOf(published))

		if modelVerifiedIn(into) {
			t.Fatal("a model that is not the published file counts as checked")
		}

		if err := Install(context.Background(), nil); err != nil {
			t.Fatal(err)
		}

		if n := asked.Load(); n != 1 {
			t.Errorf("the model was fetched %d times, want once", n)
		}

		if got := read(t, filepath.Join(into, ModelName)); got != string(published) {
			t.Errorf("the model is still %q", got)
		}

		if !modelVerifiedIn(into) {
			t.Error("the replaced model does not count as checked")
		}
	})

	t.Run("a model changed after it was checked is noticed", func(t *testing.T) {
		into := machine(t)
		os.WriteFile(filepath.Join(into, ModelName), published, 0o644)

		serveModel(t, func(w http.ResponseWriter, r *http.Request) {}, sumOf(published))

		if !modelVerifiedIn(into) {
			t.Fatal("the published model does not count as checked")
		}

		// Another size and another time: not the file that was checked.
		os.WriteFile(filepath.Join(into, ModelName), []byte("changed afterwards"), 0o644)

		if modelVerifiedIn(into) {
			t.Error("a model changed after the check still counts as checked")
		}
	})
}
