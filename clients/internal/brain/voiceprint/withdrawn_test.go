package voiceprint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

/*
 * This release offers no model that tells voices apart.
 *
 * The one 1.0.0 downloaded (CAM++, trained on VoxCeleb) was made from
 * recordings offered for research only, and the owner withdrew it on
 * 2026-11-10 until a model whose whole lineage may be used commercially
 * exists. So nothing is downloaded, a model an older release left on the
 * machine is never loaded, and asking for it says why.
 */
func TestNoSpeakerModelIsOfferedInThisRelease(t *testing.T) {
	if Offered() {
		t.Fatal("a speaker model is offered; the withdrawn one must not be")
	}

	// Nothing names the withdrawn model any more.
	for _, gone := range []string{"campplus", "voxceleb", "speaker-embedding-models"} {
		if strings.Contains(strings.ToLower(modelFrom), gone) {
			t.Errorf("the model source still names %s: %q", gone, modelFrom)
		}
	}
}

func TestInstallFetchesNothingWhenNoModelIsOffered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var asked atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		http.Error(w, "no", http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	oldFrom, oldSum := runtimeFrom, runtimeSum
	runtimeFrom, runtimeSum = srv.URL+"/onnxruntime.tgz", sumOf([]byte("lib"))

	t.Cleanup(func() { runtimeFrom, runtimeSum = oldFrom, oldSum })

	var said []string

	err := Install(context.Background(), func(s string) { said = append(said, s) })
	if !errors.Is(err, ErrWithdrawn) {
		t.Fatalf("Install returned %v, want ErrWithdrawn", err)
	}

	if n := asked.Load(); n != 0 {
		t.Errorf("Install sent %d requests with no model offered", n)
	}

	if len(said) != 0 {
		t.Errorf("Install announced a download it did not make: %q", said)
	}

	if _, err := os.Stat(Parts()); err == nil {
		t.Error("Install made the parts folder although nothing was fetched")
	}
}

/*
 * What 1.0.0 left on the machine stays there and is never loaded.
 *
 * Its files are the user's disk, not ours to delete without asking; they are
 * simply not used. The runtime refresh at start, which used to replace an
 * older runtime for somebody who had the model, sends nothing either.
 */
func TestAModelAnOlderReleaseLeftIsNeitherLoadedNorRefreshed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	into := Parts()

	if err := os.MkdirAll(into, 0o755); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(into, ModelName), []byte("the 1.0.0 model"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), []byte("lib"), 0o755)
	os.WriteFile(filepath.Join(into, RuntimeStamp), []byte("1.29.0\n"), 0o644)

	var asked atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
	}))
	t.Cleanup(srv.Close)

	oldFrom := runtimeFrom
	runtimeFrom = srv.URL + "/onnxruntime.tgz"

	t.Cleanup(func() { runtimeFrom = oldFrom })

	replaced, err := Refresh(context.Background(), nil)
	if err != nil || replaced {
		t.Errorf("Refresh = %v, %v; want nothing done", replaced, err)
	}

	if asked.Load() != 0 {
		t.Error("Refresh downloaded a runtime for a withdrawn model")
	}

	// Stamped as current, it is still not installed: there is no model to run.
	os.WriteFile(filepath.Join(into, RuntimeStamp), []byte(RuntimeVersion+"\n"), 0o644)

	if Installed() {
		t.Error("a model an older release left counts as installed")
	}

	for _, name := range []string{ModelName, RuntimeName} {
		if _, err := os.Stat(filepath.Join(into, name)); err != nil {
			t.Errorf("%s was removed; it should stay where it was: %v", name, err)
		}
	}
}
