package preflight

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

/*
 * Ollama is one fixed release, checked before it replaces anything, and a
 * newer one already here is kept.
 *
 * Until 1.0 setup fetched releases/latest/download with no checksum at all:
 * a program that runs as a service on the person's machine, taken on trust
 * from whatever was published that day. These tests hold the replacement.
 */

func TestOllamaIsPinnedToOneReleaseAndAChecksum(t *testing.T) {
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(OllamaVersion) {
		t.Fatalf("OllamaVersion %q is not one release", OllamaVersion)
	}

	// Both 64-bit Linux builds, each with a whole digest and an exact size.
	for _, arch := range []string{"amd64", "arm64"} {
		build, ok := ollamaBuilds[arch]
		if !ok {
			t.Errorf("there is no pinned Ollama build for linux/%s", arch)

			continue
		}

		if !hexDigest.MatchString(build.sum) {
			t.Errorf("the Ollama build for %s has %q, not a whole SHA-256 in lowercase hex", arch, build.sum)
		}

		// About 1.5GB; anything far from that is a typo, and the size is
		// also the download's bound.
		if build.size < 500<<20 || build.size > 4<<30 {
			t.Errorf("the Ollama build for %s is said to be %d bytes", arch, build.size)
		}

		if !strings.Contains(build.asset, "linux-"+arch) {
			t.Errorf("the Ollama build for %s is %s", arch, build.asset)
		}
	}

	if runtime.GOOS != "linux" || ollamaBuilds[runtime.GOARCH].sum == "" {
		t.Skipf("no pinned Ollama for %s/%s, which installOllama refuses", runtime.GOOS, runtime.GOARCH)
	}

	p, err := OllamaProgram()
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(p.URL, "/latest/") || !strings.Contains(p.URL, "/download/"+OllamaVersion+"/") {
		t.Errorf("Ollama comes from %s, not from the pinned release %s", p.URL, OllamaVersion)
	}

	if p.Algorithm != "sha256" || p.Sum != ollamaBuilds[runtime.GOARCH].sum || p.Size != ollamaBuilds[runtime.GOARCH].size {
		t.Errorf("OllamaProgram does not carry the pinned digest and size: %+v", p)
	}

	if err := allowed(p.URL); err != nil {
		t.Error(err)
	}

	if p.Licence == "" {
		t.Error("Ollama says nothing about its licence")
	}
}

// installOllama's own source no longer names a moving address.
func TestInstallingOllamaNamesNoMovingRelease(t *testing.T) {
	source, err := os.ReadFile("install_parts.go")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(source), "releases/latest") {
		t.Error("install_parts.go still downloads from releases/latest")
	}
}

// aMachine is a home, a PATH and an Ollama address that belong to the test,
// with the download and the service start replaced by recorders.
type aMachine struct {
	home, path, tmp string
	fetched         []Pinned
	started         []string
	fetchSays       error
}

func newMachine(t *testing.T) *aMachine {
	t.Helper()

	if runtime.GOOS != "linux" || ollamaBuilds[runtime.GOARCH].sum == "" {
		t.Skip("installOllama installs only on 64-bit Linux")
	}

	m := &aMachine{home: t.TempDir(), path: t.TempDir(), tmp: t.TempDir()}

	t.Setenv("HOME", m.home)
	t.Setenv("PATH", m.path)
	t.Setenv("TMPDIR", m.tmp)

	// Nothing answers here unless a test says so.
	nobody := httptest.NewServer(http.NotFoundHandler())
	nobody.Close()
	t.Setenv("OLLAMA_BASE_URL", nobody.URL)

	m.fetchSays = errors.New("its sha256 is not the release's, so nothing was installed")

	fetch, start := ollamaFetch, ollamaStart

	ollamaFetch = func(p Pinned, to string, w io.Writer) error {
		m.fetched = append(m.fetched, p)

		return m.fetchSays
	}

	ollamaStart = func(binary string, w io.Writer) error {
		m.started = append(m.started, binary)

		return nil
	}

	t.Cleanup(func() { ollamaFetch, ollamaStart = fetch, start })

	return m
}

// anOllama writes a program that says it is the version given, the way
// Ollama does with no server running.
func anOllama(t *testing.T, at, version string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}

	script := "#!/bin/sh\n"
	if version != "" {
		script += "echo 'Warning: could not connect to a running Ollama instance'\n" +
			"echo 'Warning: client version is " + version + "'\n"
	}

	if err := os.WriteFile(at, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return script
}

func answering(t *testing.T, version string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			http.NotFound(w, r)

			return
		}

		fmt.Fprintf(w, `{"version":%q}`, version)
	}))
	t.Cleanup(server.Close)
	t.Setenv("OLLAMA_BASE_URL", server.URL)
}

func pinnedNumber() string { return strings.TrimPrefix(OllamaVersion, "v") }

func TestANewerOllamaAlreadyHereIsUsedAndNotReplaced(t *testing.T) {
	for _, tc := range []struct {
		name    string
		set     func(t *testing.T, m *aMachine) string
		started bool
	}{
		{"on the PATH", func(t *testing.T, m *aMachine) string {
			return anOllama(t, filepath.Join(m.path, "ollama"), "99.1.0")
		}, false},
		{"answering on the address, with no program here", func(t *testing.T, m *aMachine) string {
			answering(t, "99.1.0")

			return ""
		}, false},
		{"the same release as the pinned one", func(t *testing.T, m *aMachine) string {
			return anOllama(t, filepath.Join(m.path, "ollama"), pinnedNumber())
		}, false},
		{"our own copy in ~/.local/bin, stopped", func(t *testing.T, m *aMachine) string {
			return anOllama(t, filepath.Join(m.home, ".local", "bin", "ollama"), "99.1.0")
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMachine(t)
			before := tc.set(t, m)

			var log strings.Builder

			if err := installOllama(&log); err != nil {
				t.Fatalf("installing over a newer Ollama failed: %v\n%s", err, log.String())
			}

			if len(m.fetched) > 0 {
				t.Errorf("a newer Ollama was here and %s was downloaded to replace it", m.fetched[0].Name)
			}

			if !strings.Contains(log.String(), "already here") || !strings.Contains(log.String(), "used as it is") {
				t.Errorf("it does not say it used the one already here:\n%s", log.String())
			}

			if tc.started != (len(m.started) == 1) {
				t.Errorf("started %v; want started: %v", m.started, tc.started)
			}

			for _, at := range []string{filepath.Join(m.path, "ollama"), filepath.Join(m.home, ".local", "bin", "ollama")} {
				if got, err := os.ReadFile(at); err == nil && string(got) != before {
					t.Errorf("%s was changed", at)
				}
			}
		})
	}
}

func TestAnOlderOllamaIsReplacedOnlyByTheCheckedPinnedRelease(t *testing.T) {
	for _, tc := range []struct{ name, version string }{
		{"older", "0.30.0"},
		{"one that cannot say its version", ""},
		{"none at all", "-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMachine(t)
			ours := filepath.Join(m.home, ".local", "bin", "ollama")

			var before string
			if tc.version != "-" {
				before = anOllama(t, ours, tc.version)
			}

			var log strings.Builder

			err := installOllama(&log)

			// The download is asked for exactly once, and it is the pinned
			// release with its digest and size.
			want, _ := OllamaProgram()
			if len(m.fetched) != 1 || m.fetched[0] != want {
				t.Fatalf("fetched %+v, want only the pinned %+v", m.fetched, want)
			}

			// The recorder refused it as Fetch refuses a file whose digest
			// differs: the install fails and nothing was replaced or left.
			if err == nil || !strings.Contains(err.Error(), "nothing was installed") {
				t.Errorf("an archive that failed its check was not refused: %v", err)
			}

			if len(m.started) > 0 {
				t.Errorf("started %v after a refused download", m.started)
			}

			if tc.version != "-" {
				if got, _ := os.ReadFile(ours); string(got) != before {
					t.Error("the Ollama that was here changed although the download was refused")
				}
			} else if _, err := os.Stat(filepath.Join(m.home, ".local", "lib")); err == nil {
				t.Error("something was unpacked although the download was refused")
			}

			left, _ := filepath.Glob(filepath.Join(m.tmp, "pn-scripts-assistant-ollama-*"))
			if len(left) > 0 {
				t.Errorf("left in the temporary folder: %v", left)
			}
		})
	}
}

func TestTheNewestOllamaHereIsTheOneThatCounts(t *testing.T) {
	m := newMachine(t)

	anOllama(t, filepath.Join(m.path, "ollama"), "0.30.0")
	anOllama(t, filepath.Join(m.home, ".local", "bin", "ollama"), "0.41.2")
	answering(t, "0.29.0")

	got := ollamaAtHand()

	if got.version != "0.41.2" || got.binary != filepath.Join(m.home, ".local", "bin", "ollama") {
		t.Errorf("found %+v, want the 0.41.2 in ~/.local/bin", got)
	}

	// The release being compared numerically: 0.40.11 is newer than 0.40.9.
	if !IsNewer("0.40.11", "0.40.9") || IsNewer("0.40.9", "0.40.11") {
		t.Error("versions are not compared as numbers")
	}
}

// A program asked while a server of another version runs names the server
// first; its own version is the "client version" line.
func TestAnOllamaProgramIsKnownByItsOwnVersion(t *testing.T) {
	at := filepath.Join(t.TempDir(), "ollama")

	for _, tc := range []struct{ said, want string }{
		{"ollama version is 0.34.0\nWarning: client version is 0.40.11", "0.40.11"},
		{"ollama version is 0.40.11", "0.40.11"},
		{"Warning: could not connect to a running Ollama instance\nWarning: client version is 0.40.11", "0.40.11"},
		{"something else", ""},
	} {
		os.WriteFile(at, []byte("#!/bin/sh\nprintf '%s\\n' '"+strings.ReplaceAll(tc.said, "\n", "' '")+"'\n"), 0o755)

		if got := ollamaBinaryVersion(at); got != tc.want {
			t.Errorf("%q was read as %q, want %q", tc.said, got, tc.want)
		}
	}
}

/*
 * Fetching the pinned release for real and asking it which version it is.
 *
 * Downloads 1.5GB, so it runs only with PN_SCRIPTS_ASSISTANT_LIVE_INSTALL set
 * (and TMPDIR on a disk with room). It goes no further than unpacking into a
 * folder of the test's own: the service is the person's, never started here.
 */
func TestFetchingTheOllamaReleaseForReal(t *testing.T) {
	if os.Getenv("PN_SCRIPTS_ASSISTANT_LIVE_INSTALL") == "" {
		t.Skip("set PN_SCRIPTS_ASSISTANT_LIVE_INSTALL=1 to download the pinned Ollama for real")
	}

	p, err := OllamaProgram()
	if err != nil {
		t.Skip(err)
	}

	dir := t.TempDir()
	archive := filepath.Join(dir, "ollama.tar.zst")

	var log strings.Builder

	if err := Fetch(p, archive, &log); err != nil {
		t.Fatalf("the pinned Ollama was refused: %v\n%s", err, log.String())
	}

	plain := filepath.Join(dir, "ollama.tar")

	if err := run(&log, []string{"unzstd", "-q", "-f", "-o", plain, archive}); err != nil {
		t.Fatalf("unzstd: %v\n%s", err, log.String())
	}

	into := filepath.Join(dir, "local")

	if err := unpackTar(plain, into, 0, io.Discard); err != nil {
		t.Fatal(err)
	}

	if got := ollamaBinaryVersion(filepath.Join(into, "bin", "ollama")); got != pinnedNumber() {
		t.Errorf("the unpacked Ollama says it is %q, want %s", got, pinnedNumber())
	}
}
