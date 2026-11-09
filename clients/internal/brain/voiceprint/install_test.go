package voiceprint

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

/*
 * The runtime download: what it accepts, what it refuses, and what an update
 * of the program does to a runtime fetched by an older one.
 *
 * Nothing here reaches the internet. Each test serves its own archive from
 * this machine, laid out like the real release (links, the versioned library,
 * the providers library beside it), and points the installer at it.
 */

// releaseArchive builds a .tgz shaped like onnxruntime-linux-x64-<version>.tgz
// with library as the versioned file's contents.
func releaseArchive(t *testing.T, version string, library []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	dir := "onnxruntime-linux-x64-" + version + "/lib/"

	add := func(h *tar.Header, body []byte) {
		if err := archive.WriteHeader(h); err != nil {
			t.Fatal(err)
		}

		if _, err := archive.Write(body); err != nil {
			t.Fatal(err)
		}
	}

	add(&tar.Header{Name: dir + "libonnxruntime.so.1", Typeflag: tar.TypeSymlink,
		Linkname: "libonnxruntime.so." + version}, nil)
	add(&tar.Header{Name: dir + "libonnxruntime.so", Typeflag: tar.TypeSymlink,
		Linkname: "libonnxruntime.so.1"}, nil)
	add(&tar.Header{Name: dir + "libonnxruntime_providers_shared.so", Typeflag: tar.TypeReg,
		Mode: 0o755, Size: 9}, []byte("providers"))
	add(&tar.Header{Name: dir + "libonnxruntime.so." + version, Typeflag: tar.TypeReg,
		Mode: 0o755, Size: int64(len(library))}, library)

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func sumOf(b []byte) string {
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// serve points the installer at a local server handing out body, expecting
// the SHA-256 want, and counts the requests it gets.
func serve(t *testing.T, body []byte, want string) *atomic.Int32 {
	t.Helper()

	var asked atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	oldFrom, oldSum := runtimeFrom, runtimeSum
	runtimeFrom, runtimeSum = srv.URL+"/onnxruntime.tgz", want

	t.Cleanup(func() { runtimeFrom, runtimeSum = oldFrom, oldSum })

	return &asked
}

// machine gives the test its own home, so Parts() is a temporary folder and
// never the real one, and returns that folder, made.
func machine(t *testing.T) string {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	/*
	 * In these tests the bytes "model" are the published model, so a folder
	 * a test lays out as already installed is one. A test about the model
	 * download itself names its own (serveModel).
	 */
	oldSum := modelSum
	modelSum = sumOf([]byte("model"))

	t.Cleanup(func() { modelSum = oldSum })

	into := Parts()

	if err := os.MkdirAll(into, 0o755); err != nil {
		t.Fatal(err)
	}

	return into
}

func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

// leftovers lists anything in the folder besides the names given.
func leftovers(t *testing.T, into string, expected ...string) []string {
	t.Helper()

	entries, err := os.ReadDir(into)
	if err != nil {
		t.Fatal(err)
	}

	var extra []string

	for _, e := range entries {
		found := false

		for _, name := range expected {
			found = found || e.Name() == name
		}

		if !found {
			extra = append(extra, e.Name())
		}
	}

	return extra
}

func TestThePinnedRuntimeIsWellFormed(t *testing.T) {
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(RuntimeSHA256) {
		t.Errorf("RuntimeSHA256 %q is not a SHA-256 in lowercase hex", RuntimeSHA256)
	}

	if !strings.HasSuffix(RuntimeFrom, "/v"+RuntimeVersion+"/onnxruntime-linux-x64-"+RuntimeVersion+".tgz") {
		t.Errorf("RuntimeFrom %q does not name release %s", RuntimeFrom, RuntimeVersion)
	}

	if runtimeFrom != RuntimeFrom || runtimeSum != RuntimeSHA256 {
		t.Error("the download source differs from the published constants outside a test")
	}
}

/*
 * The binding and the runtime have to be from the same release.
 *
 * The binding asks the library for one version of its C interface
 * (ORT_API_VERSION in the header it ships); onnxruntime 1.N serves version N.
 * This reads the header of the binding this module actually builds with, so a
 * dependency update that moves the binding without moving RuntimeVersion
 * (Dependabot's pull request on its own) fails here instead of reaching
 * somebody whose voiceprint then stops working.
 */
func TestTheRuntimeMatchesTheBinding(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/yalue/onnxruntime_go").Output()
	if err != nil {
		t.Fatalf("finding the onnxruntime_go module: %v", err)
	}

	header, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "onnxruntime_c_api.h"))
	if err != nil {
		t.Fatalf("reading the binding's header: %v", err)
	}

	found := regexp.MustCompile(`(?m)^#define ORT_API_VERSION (\d+)\s*$`).FindSubmatch(header)
	if found == nil {
		t.Fatal("the binding's header does not define ORT_API_VERSION")
	}

	if want := "1." + string(found[1]) + "."; !strings.HasPrefix(RuntimeVersion, want) {
		t.Errorf("the binding speaks ORT_API_VERSION %s, which is onnxruntime %sx; RuntimeVersion is %s",
			found[1], want, RuntimeVersion)
	}
}

func TestTheRuntimeIsInstalledWhenItIsThePublishedOne(t *testing.T) {
	into := machine(t)
	library := []byte("the real library, " + RuntimeVersion)
	archive := releaseArchive(t, RuntimeVersion, library)

	serve(t, archive, sumOf(archive))

	if err := runtime(context.Background(), into); err != nil {
		t.Fatalf("the published archive was refused: %v", err)
	}

	if got := read(t, filepath.Join(into, RuntimeName)); got != string(library) {
		t.Errorf("installed %q, want the versioned library %q", got, library)
	}

	if got := read(t, filepath.Join(into, RuntimeStamp)); got != RuntimeVersion+"\n" {
		t.Errorf("stamp says %q, want %q", got, RuntimeVersion)
	}

	if !RuntimeCurrent() || RuntimeStale() {
		t.Error("a freshly installed runtime does not read as current")
	}

	// The archive, the providers library and the links are not left behind.
	if extra := leftovers(t, into, RuntimeName, RuntimeStamp); len(extra) > 0 {
		t.Errorf("left behind: %v", extra)
	}
}

func TestATamperedRuntimeIsRefused(t *testing.T) {
	into := machine(t)
	published := releaseArchive(t, RuntimeVersion, []byte("the real library"))
	changed := releaseArchive(t, RuntimeVersion, []byte("something else entirely"))

	serve(t, changed, sumOf(published))

	err := runtime(context.Background(), into)

	if !errors.Is(err, ErrRuntimeTampered) {
		t.Fatalf("a changed archive gave %v, want ErrRuntimeTampered", err)
	}

	if !strings.Contains(err.Error(), sumOf(changed)) || !strings.Contains(err.Error(), sumOf(published)) {
		t.Errorf("the refusal does not name both hashes: %v", err)
	}

	// Nothing of it on the disk: no library, no stamp, no half-written file.
	if extra := leftovers(t, into); len(extra) > 0 {
		t.Errorf("a refused download left %v", extra)
	}

	if RuntimeCurrent() {
		t.Error("a refused download reads as installed")
	}
}

func TestATamperedRuntimeNeverReplacesTheOneThere(t *testing.T) {
	into := machine(t)
	old := []byte("the library from 1.29.0")

	os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), old, 0o755)

	published := releaseArchive(t, RuntimeVersion, []byte("the real library"))
	changed := releaseArchive(t, RuntimeVersion, []byte("something else"))

	serve(t, changed, sumOf(published))

	if _, err := Refresh(context.Background(), nil); !errors.Is(err, ErrRuntimeTampered) {
		t.Fatalf("Refresh with a changed archive gave %v", err)
	}

	if got := read(t, filepath.Join(into, RuntimeName)); got != string(old) {
		t.Errorf("the library on disk became %q", got)
	}

	if RuntimeCurrent() {
		t.Error("a refused refresh made the old runtime read as current")
	}
}

func TestAnEndlessRuntimeDownloadIsCutOff(t *testing.T) {
	into := machine(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1<<20)

		for i := 0; i <= mostRuntimeBytes>>20+1; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	oldFrom := runtimeFrom
	runtimeFrom = srv.URL

	defer func() { runtimeFrom = oldFrom }()

	if err := runtime(context.Background(), into); !errors.Is(err, ErrRuntimeTampered) {
		t.Fatalf("an oversized download gave %v", err)
	}

	if extra := leftovers(t, into); len(extra) > 0 {
		t.Errorf("an oversized download left %v", extra)
	}
}

func TestAnArchiveWithoutTheVersionedLibraryIsRefused(t *testing.T) {
	into := machine(t)

	// Published-looking, hash right, but from another release: the file
	// inside is libonnxruntime.so.1.29.0, not the one this copy needs.
	archive := releaseArchive(t, "1.29.0", []byte("old"))

	serve(t, archive, sumOf(archive))

	if err := runtime(context.Background(), into); err == nil {
		t.Fatal("an archive of another release was installed")
	}

	if RuntimeCurrent() {
		t.Error("it reads as current")
	}
}

/*
 * The upgrade: a runtime fetched by an older release is replaced.
 *
 * Copies before this one wrote the library and no stamp, so "no stamp" is
 * exactly how a 1.29.0 library from an earlier install looks. A stamp naming
 * another release is the same case one update later.
 */
func TestAnOlderRuntimeIsReplacedOnUpgrade(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stamp string // "" for no stamp file at all
	}{
		{"installed before stamps existed", ""},
		{"stamped by an older release", "1.29.0\n"},
		{"stamp unreadable as a version", "garbage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			into := machine(t)

			os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)
			os.WriteFile(filepath.Join(into, RuntimeName), []byte("the library from 1.29.0"), 0o755)

			if tc.stamp != "" {
				os.WriteFile(filepath.Join(into, RuntimeStamp), []byte(tc.stamp), 0o644)
			}

			if !RuntimeStale() || RuntimeCurrent() {
				t.Fatal("the old runtime is not recognised as from another release")
			}

			library := []byte("the library from " + RuntimeVersion)
			archive := releaseArchive(t, RuntimeVersion, library)
			asked := serve(t, archive, sumOf(archive))

			var said []string

			replaced, err := Refresh(context.Background(), func(s string) { said = append(said, s) })
			if err != nil || !replaced {
				t.Fatalf("Refresh = %v, %v; want the runtime replaced", replaced, err)
			}

			if got := read(t, filepath.Join(into, RuntimeName)); got != string(library) {
				t.Errorf("the library is still %q", got)
			}

			if !RuntimeCurrent() || RuntimeStale() {
				t.Error("after the refresh it does not read as current")
			}

			if len(said) == 0 {
				t.Error("the refresh ran without saying so")
			}

			// The model was not fetched again, and a second start does nothing.
			if got := read(t, filepath.Join(into, ModelName)); got != "model" {
				t.Errorf("the model was touched: %q", got)
			}

			if replaced, err := Refresh(context.Background(), nil); replaced || err != nil {
				t.Errorf("a second Refresh = %v, %v; want nothing to do", replaced, err)
			}

			if n := asked.Load(); n != 1 {
				t.Errorf("the runtime was requested %d times, want once", n)
			}
		})
	}
}

func TestInstallFetchesOnlyTheRuntimeWhenItIsOld(t *testing.T) {
	into := machine(t)

	os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), []byte("old"), 0o755)

	library := []byte("new")
	archive := releaseArchive(t, RuntimeVersion, library)
	asked := serve(t, archive, sumOf(archive))

	if err := Install(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if got := read(t, filepath.Join(into, RuntimeName)); got != "new" {
		t.Errorf("Install kept the old runtime: %q", got)
	}

	if n := asked.Load(); n != 1 {
		t.Errorf("runtime requested %d times", n)
	}
}

func TestRefreshSendsNothingForSomebodyWhoNeverTurnedItOn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{"nothing installed", nil},
		{"only the model", []string{ModelName}},
		{"a runtime without the model", []string{RuntimeName}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			into := machine(t)

			for _, f := range tc.files {
				os.WriteFile(filepath.Join(into, f), []byte("x"), 0o644)
			}

			asked := serve(t, nil, RuntimeSHA256)

			if replaced, err := Refresh(context.Background(), nil); replaced || err != nil {
				t.Errorf("Refresh = %v, %v", replaced, err)
			}

			if n := asked.Load(); n != 0 {
				t.Errorf("%d requests were made", n)
			}
		})
	}
}

func TestACurrentRuntimeIsLeftAlone(t *testing.T) {
	into := machine(t)

	os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), []byte("current"), 0o755)
	os.WriteFile(filepath.Join(into, RuntimeStamp), []byte(RuntimeVersion+"\n"), 0o644)

	asked := serve(t, nil, RuntimeSHA256)

	if replaced, err := Refresh(context.Background(), nil); replaced || err != nil {
		t.Errorf("Refresh = %v, %v", replaced, err)
	}

	if err := Install(context.Background(), nil); err != nil {
		t.Errorf("Install with everything current: %v", err)
	}

	if n := asked.Load(); n != 0 {
		t.Errorf("%d requests were made for a current runtime", n)
	}
}
