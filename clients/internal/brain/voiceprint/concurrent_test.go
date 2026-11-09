package voiceprint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

/*
 * The runtime refresh at start and the Install button, at the same time.
 *
 * Both fetched into the same fixed file (onnxruntime.tgz.part) and wrote the
 * library through the same fixed name (<library>.part), so a refresh still
 * downloading when somebody pressed the button had its archive truncated
 * under it: the review's run failed five times in six with a hash mismatch or
 * a missing file. Now one fetch at a time, each into a file of its own, and
 * whoever waited finds the work done instead of doing it again.
 */
func TestRefreshAndInstallAtOnceFetchTheRuntimeOnce(t *testing.T) {
	into := machine(t)

	os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), []byte("the library from 1.29.0"), 0o755)

	library := []byte("the library from " + RuntimeVersion)
	archive := releaseArchive(t, RuntimeVersion, library)

	var asked atomic.Int32

	// Slow enough that every caller below is waiting while the first is
	// still receiving.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)

		half := len(archive) / 2
		w.Write(archive[:half])
		w.(http.Flusher).Flush()
		time.Sleep(150 * time.Millisecond)
		w.Write(archive[half:])
	}))
	t.Cleanup(srv.Close)

	oldFrom, oldSum := runtimeFrom, runtimeSum
	runtimeFrom, runtimeSum = srv.URL, sumOf(archive)

	t.Cleanup(func() { runtimeFrom, runtimeSum = oldFrom, oldSum })

	var wg sync.WaitGroup

	errs := make(chan error, 6)

	for i := 0; i < 3; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()

			_, err := Refresh(context.Background(), nil)
			errs <- err
		}()

		go func() {
			defer wg.Done()

			errs <- Install(context.Background(), nil)
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("a refresh or install beside another failed: %v", err)
		}
	}

	if n := asked.Load(); n != 1 {
		t.Errorf("the runtime was downloaded %d times, want once", n)
	}

	if got := read(t, filepath.Join(into, RuntimeName)); got != string(library) {
		t.Errorf("the library is %q, want the new one whole", got)
	}

	if !RuntimeCurrent() {
		t.Error("after all of them the runtime does not read as current")
	}

	if extra := leftovers(t, into, ModelName, RuntimeName, RuntimeStamp); len(extra) > 0 {
		t.Errorf("left behind: %v", extra)
	}
}
