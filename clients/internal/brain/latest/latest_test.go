package latest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyALaterReleaseIsNewer(t *testing.T) {
	cases := []struct {
		have, latest string
		newer        bool
	}{
		{"0.11.0", "0.12.0", true},
		{"0.12.0", "0.12.0", false},
		{"0.12.1", "0.12.0", false},
		{"0.9.9", "0.10.0", true},
		{"1.0.0", "0.99.0", false},
		{"v0.11.0", "v0.11.1", true},
		// A build after a release is that release plus work: never "update"
		// to what it already contains.
		{"0.12.0~git20261108140320.8aa0f34", "0.12.0", false},
		{"0.11.0~git20261108140320.8aa0f34", "0.12.0", true},
		// Nothing comparable is never newer.
		{"built from source", "0.12.0", false},
		{"0.12.0", "latest", false},
		{"", "0.12.0", false},
	}

	for _, c := range cases {
		if got := Newer(c.have, c.latest); got != c.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.have, c.latest, got, c.newer)
		}
	}
}

func TestTheChannelFollowsHowItWasInstalled(t *testing.T) {
	present := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, q := range paths {
				if filepath.ToSlash(p) == q {
					return true
				}
			}

			return false
		}
	}

	// Stands for "dpkg-query says the package is installed".
	dpkg := "dpkg-query"

	cases := []struct {
		name, version, goos, exe, snap string
		files                          []string
		want                           Channel
	}{
		{"built by hand", "", "linux", "/home/you/src/dist/pn-scripts-assistant", "", nil, Source},
		{"deb with its repository", "0.12.0", "linux", "/usr/bin/pn-scripts-assistant", "", []string{dpkg, AptSources}, Apt},
		{"deb, repository removed", "0.12.0", "linux", "/usr/bin/pn-scripts-assistant", "", []string{dpkg}, Deb},
		{"a binary someone copied to /usr/bin", "0.12.0", "linux", "/usr/bin/pn-scripts-assistant", "", nil, Portable},
		{"snap", "0.12.0", "linux", "/snap/pn-scripts-assistant/12/bin/pn-scripts-assistant", "pn-scripts-assistant", nil, Snap},
		{"loose linux binary", "0.12.0", "linux", "/home/you/Downloads/pn-scripts-assistant-linux-amd64", "", nil, Portable},
		{"windows installer", "0.12.0", "windows", "/c/Users/you/AppData/Local/Programs/PN Scripts Assistant/pn-scripts-assistant.exe", "",
			[]string{"/c/Users/you/AppData/Local/Programs/PN Scripts Assistant/unins000.exe"}, Installer},
		{"windows zip", "0.12.0", "windows", "/e/PN-Scripts-Assistant/pn-scripts-assistant.exe", "", nil, Portable},
	}

	for _, c := range cases {
		has := present(c.files...)

		if got := how(c.version, c.goos, c.exe, c.snap, has, func() bool { return has(dpkg) }); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestASumIsReadOnlyForItsOwnFile(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	sums := sum + "  pn-scripts-assistant-0.12.0-windows-setup.exe\n" +
		strings.Repeat("cd", 32) + " *pn-scripts-assistant_0.12.0_amd64.deb\n" +
		"nonsense line\n"

	got, err := SumFor(sums, "pn-scripts-assistant-0.12.0-windows-setup.exe")
	if err != nil || got != sum {
		t.Fatalf("got %q, %v", got, err)
	}

	if got, err := SumFor(sums, "pn-scripts-assistant_0.12.0_amd64.deb"); err != nil || got != strings.Repeat("cd", 32) {
		t.Fatalf("binary-mode line: got %q, %v", got, err)
	}

	if _, err := SumFor(sums, "pn-scripts-assistant-0.12.0-windows-amd64.zip"); err == nil {
		t.Fatal("a file the sums do not name must not get a sum")
	}

	if _, err := SumFor("xyz  file\n", "file"); err == nil {
		t.Fatal("a line that is not a SHA-256 must not count")
	}
}

// fakeRelease serves a release the way GitHub does: the API answer, the file
// and its SHA256SUMS — with the sums possibly lying.
func fakeRelease(t *testing.T, version string, file []byte, lie bool) *httptest.Server {
	t.Helper()

	name := "pn-scripts-assistant-" + version + "-windows-setup.exe"
	sum := sha256.Sum256(file)
	listed := hex.EncodeToString(sum[:])

	if lie {
		listed = strings.Repeat("0", 64)
	}

	mux := http.NewServeMux()

	var srv *httptest.Server

	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != "pn-scripts-assistant" {
			t.Errorf("the check said more about itself than its name: %q", ua)
		}

		fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"https://github.com/pnscripts/pn-scripts-assistant/releases/tag/v%s",
			"assets":[{"name":%q,"browser_download_url":"%s/dl/%s","size":%d},
			{"name":"SHA256SUMS","browser_download_url":"%s/dl/SHA256SUMS","size":100}]}`,
			version, version, name, srv.URL, name, len(file), srv.URL)
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(file) })
	mux.HandleFunc("/dl/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", listed, name)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv(TestEnv, srv.URL+"/latest")

	return srv
}

func TestTheNewestReleaseIsReadAndItsFileChecked(t *testing.T) {
	file := []byte("pretend installer")
	fakeRelease(t, "0.99.0", file, false)

	rel, err := Newest(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if rel.Version != "0.99.0" || !strings.HasSuffix(rel.Page, "/v0.99.0") {
		t.Fatalf("read %+v", rel)
	}

	dir := t.TempDir()

	path, err := Fetch(context.Background(), nil, rel, "pn-scripts-assistant-0.99.0-windows-setup.exe", dir)
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(path); string(got) != string(file) {
		t.Fatalf("downloaded %q", got)
	}
}

func TestAFileThatDoesNotMatchItsSumIsDeleted(t *testing.T) {
	fakeRelease(t, "0.99.0", []byte("tampered"), true)

	rel, err := Newest(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	if _, err := Fetch(context.Background(), nil, rel, "pn-scripts-assistant-0.99.0-windows-setup.exe", dir); err == nil {
		t.Fatal("a file whose sum does not match was accepted")
	}

	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestDownloadsComeOnlyFromThisProgramsReleases(t *testing.T) {
	ok := []string{
		"https://github.com/pnscripts/pn-scripts-assistant/releases/download/v0.12.0/SHA256SUMS",
		"https://release-assets.githubusercontent.com/github-production-release-asset/1/2",
		"https://api.github.com/repos/pnscripts/pn-scripts-assistant/releases/latest",
	}

	refused := []string{
		"http://github.com/pnscripts/pn-scripts-assistant/releases/download/v0.12.0/SHA256SUMS",
		"https://github.com/someone/else/releases/download/v1/setup.exe",
		"https://evilgithubusercontent.com/x",
		"https://example.org/setup.exe",
		// Loopback is only for the tests' own override.
		"http://127.0.0.1:8080/setup.exe",
	}

	for _, raw := range ok {
		u, _ := url.Parse(raw)
		if err := allowed(u, false); err != nil {
			t.Errorf("%s refused: %v", raw, err)
		}
	}

	for _, raw := range refused {
		u, _ := url.Parse(raw)
		if err := allowed(u, false); err == nil {
			t.Errorf("%s allowed", raw)
		}
	}
}

func TestTheOverrideIsHonouredOnlyForThisMachine(t *testing.T) {
	t.Setenv(TestEnv, "https://example.org/latest")

	if from, testing := source(); testing || from != api {
		t.Fatalf("a remote override was taken: %s", from)
	}

	t.Setenv(TestEnv, "http://127.0.0.1:9/latest")

	if from, testing := source(); !testing || from != "http://127.0.0.1:9/latest" {
		t.Fatalf("the loopback override was not taken: %s", from)
	}
}

func TestOfflineIsAnAnswerNotAFault(t *testing.T) {
	// Port 9 on loopback: nothing listens there.
	t.Setenv(TestEnv, "http://127.0.0.1:9/latest")

	_, err := Newest(context.Background(), nil)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("got %v, want ErrOffline", err)
	}
}

func TestTheInstallerIsStartedOnlyAfterItsCheck(t *testing.T) {
	file := []byte("pretend installer")
	fakeRelease(t, "0.99.0", file, false)

	var ran []string

	was := start
	start = func(name string, args ...string) error {
		ran = append([]string{name}, args...)

		return nil
	}

	t.Cleanup(func() { start = was })

	rel, err := Newest(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	out, err := Apply(context.Background(), Installer, rel, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if !out.Started || len(ran) == 0 || filepath.Base(ran[0]) != "pn-scripts-assistant-0.99.0-windows-setup.exe" {
		t.Fatalf("outcome %+v, ran %v", out, ran)
	}

	if !strings.Contains(strings.Join(ran, " "), "/RELAUNCH=1") {
		t.Fatalf("the installer would not open the program again: %v", ran)
	}

	// And a lying sum starts nothing.
	ran = nil

	fakeRelease(t, "0.99.1", []byte("tampered"), true)

	rel, _ = Newest(context.Background(), nil)

	if _, err := Apply(context.Background(), Installer, rel, t.TempDir()); err == nil || ran != nil {
		t.Fatalf("a file that failed its check was started: %v %v", err, ran)
	}
}

func TestAPackageManagerCopyIsLeftToThePackageManager(t *testing.T) {
	rel := Release{Version: "0.99.0", Page: Page}

	wasLook, wasStart := lookPath, start

	t.Cleanup(func() { lookPath, start = wasLook, wasStart })

	var started string

	start = func(name string, args ...string) error { started = name; return nil }

	lookPath = func(string) (string, error) { return "/usr/bin/update-manager", nil }

	out, err := Apply(context.Background(), Apt, rel, t.TempDir())
	if err != nil || out.Opened == "" || started != "/usr/bin/update-manager" {
		t.Fatalf("apt with Software Updater: %+v %v %q", out, err, started)
	}

	lookPath = func(string) (string, error) { return "", errors.New("not here") }

	out, err = Apply(context.Background(), Apt, rel, t.TempDir())
	if err != nil || out.Command != "sudo apt update && sudo apt upgrade" {
		t.Fatalf("apt without Software Updater: %+v %v", out, err)
	}

	for _, c := range []Channel{Deb, Portable} {
		out, err := Apply(context.Background(), c, rel, t.TempDir())
		if err != nil || out.Open != Page || out.Started {
			t.Fatalf("%s: %+v %v", c, out, err)
		}
	}

	if _, err := Apply(context.Background(), Source, rel, t.TempDir()); err == nil {
		t.Fatal("a copy built by hand was 'updated' from a release")
	}
}

func TestAnUpdateStaysWhereTheFirstInstallPutIt(t *testing.T) {
	local := filepath.Join("/c", "Users", "you", "AppData", "Local")

	if got := scopeArg(filepath.Join(local, "Programs", "PN Scripts Assistant", "pn-scripts-assistant.exe"), local); got != "/CURRENTUSER" {
		t.Errorf("per-user install: %s", got)
	}

	if got := scopeArg(filepath.Join("/c", "Program Files", "PN Scripts Assistant", "pn-scripts-assistant.exe"), local); got != "/ALLUSERS" {
		t.Errorf("machine-wide install: %s", got)
	}

	// A folder merely starting with the same letters is not inside it.
	if got := scopeArg(local+"Other"+string(filepath.Separator)+"x.exe", local); got != "/ALLUSERS" {
		t.Errorf("look-alike folder: %s", got)
	}
}
