package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

/*
 * The speech parts are fixed files, checked before they are used.
 *
 * Until 1.0 the voices and the speech model came from the main branch of their
 * repositories and the Piper program, which is run, had no checksum at all;
 * whisper.cpp was compiled from whatever its main branch held that day. These
 * tests hold the replacement: one commit or release each, a SHA-256 for every
 * file, a size bound on every download, and a file that is not the published
 * one never written under its real name.
 */

func sha(b []byte) string {
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

var hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestEverySpeechPartIsPinnedToACommitAndAChecksum(t *testing.T) {
	parts := append(PiperVoices(), SpeechModelPinned())

	if runtime.GOOS == "linux" {
		if p, err := PiperProgram(); err == nil {
			parts = append(parts, p)
		}
	}

	// The three voices, a model and a settings file each.
	if n := len(PiperVoices()); n != 6 {
		t.Errorf("%d voice files are pinned, want 6 (three voices, two files each)", n)
	}

	for _, p := range parts {
		if strings.Contains(p.URL, "/resolve/main/") || strings.Contains(p.URL, "/latest/") {
			t.Errorf("%s comes from a branch, not a fixed commit or release: %s", p.Name, p.URL)
		}

		if p.Version == "" || !strings.Contains(p.URL, p.Version) {
			t.Errorf("%s: the address %s does not name the pinned revision %q", p.Name, p.URL, p.Version)
		}

		if p.Algorithm != "sha256" || !hexDigest.MatchString(p.Sum) {
			t.Errorf("%s: %q is not a whole SHA-256 in lowercase hex", p.Name, p.Sum)
		}

		if p.Size <= 0 {
			t.Errorf("%s has no size, so its download has no exact bound", p.Name)
		}

		if err := allowed(p.URL); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}

		if p.Licence == "" {
			t.Errorf("%s says nothing about its licence", p.Name)
		}
	}

	if SpeechModelFrom != "https://huggingface.co/ggerganov/whisper.cpp/resolve/"+SpeechModelRevision+"/"+SpeechModel {
		t.Errorf("SpeechModelFrom is %s", SpeechModelFrom)
	}

	// Every build of the program that is run is pinned for its platform.
	for arch, build := range piperBuilds {
		if !hexDigest.MatchString(build.sum) || build.size <= 0 {
			t.Errorf("the Piper build for %s has no whole checksum or size", arch)
		}
	}

	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(WhisperCommit) {
		t.Errorf("WhisperCommit %q is not a whole commit id", WhisperCommit)
	}

	if whisperSource.repo != WhisperRepo || whisperSource.tag != WhisperTag || whisperSource.commit != WhisperCommit {
		t.Error("the whisper.cpp source differs from the published constants outside a test")
	}
}

// pinnedServer hands out body and counts the requests it gets.
func pinnedServer(t *testing.T, body []byte, length bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var asked atomic.Int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)

		if !length {
			// Streamed, so the server never says how big it is.
			w.Header().Set("Transfer-Encoding", "chunked")
		}

		w.Write(body)
	}))
	t.Cleanup(server.Close)

	return server, &asked
}

func TestAPinnedFileThatIsNotThePublishedOneWritesNothing(t *testing.T) {
	published := []byte("the published voice")
	changed := []byte("something else entirely")

	server, _ := pinnedServer(t, changed, true)

	dir := t.TempDir()
	into := filepath.Join(dir, "en_GB-alba-medium.onnx")

	// What a working install already has there must survive a bad download.
	os.WriteFile(into, published, 0o644)

	err := downloadVia(server.Client(), server.URL+"/voice.onnx", into,
		expect{algorithm: "sha256", sum: sha(published), most: 1 << 20}, io.Discard)
	if err == nil {
		t.Fatal("a file that is not the published one was accepted")
	}

	// Says why, with both digests, and that nothing was installed.
	for _, want := range []string{sha(changed), sha(published), "nothing was installed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}

	if got, _ := os.ReadFile(into); string(got) != string(published) {
		t.Errorf("the file there became %q", got)
	}

	if parts := partFiles(t, dir); len(parts) > 0 {
		t.Errorf("a refused download left %v", parts)
	}
}

func TestAPinnedFileThatIsThePublishedOneIsInstalled(t *testing.T) {
	published := []byte("the published voice")

	server, _ := pinnedServer(t, published, true)

	dir := t.TempDir()
	into := filepath.Join(dir, "voice.onnx")

	if err := downloadVia(server.Client(), server.URL+"/voice.onnx", into,
		expect{algorithm: "sha256", sum: sha(published), most: int64(len(published))}, io.Discard); err != nil {
		t.Fatalf("the published file was refused: %v", err)
	}

	if got, _ := os.ReadFile(into); string(got) != string(published) {
		t.Errorf("installed %q", got)
	}

	if parts := partFiles(t, dir); len(parts) > 0 {
		t.Errorf("left beside it: %v", parts)
	}
}

/*
 * No download is unbounded.
 *
 * A server that never stops sending, or sends more than the file it was asked
 * for, is cut off at the bound and nothing is kept: with the size it says up
 * front, before anything is written; without one, one byte past the bound.
 */
func TestADownloadLargerThanItMayBeIsCutOff(t *testing.T) {
	body := make([]byte, 64<<10)

	for _, tc := range []struct {
		name   string
		length bool
	}{
		{"saying its size", true},
		{"not saying its size", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := pinnedServer(t, body, tc.length)

			dir := t.TempDir()
			into := filepath.Join(dir, "piper.tgz")

			err := downloadVia(server.Client(), server.URL+"/piper.tgz", into,
				expect{most: int64(len(body)) - 1}, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "more than") && !strings.Contains(err.Error(), "larger than") {
				t.Fatalf("a download past its bound gave %v", err)
			}

			if _, err := os.Stat(into); err == nil {
				t.Error("an oversized download was given its name")
			}

			if parts := partFiles(t, dir); len(parts) > 0 {
				t.Errorf("an oversized download left %v", parts)
			}
		})
	}

	// Exactly the bound is not past it.
	server, _ := pinnedServer(t, body, false)

	if err := downloadVia(server.Client(), server.URL+"/x", filepath.Join(t.TempDir(), "x"),
		expect{most: int64(len(body))}, io.Discard); err != nil {
		t.Errorf("a download of exactly its bound was refused: %v", err)
	}

	// And a download with nothing written down still has one.
	if (expect{}).limit() != mostBytes || mostBytes > 8<<30 {
		t.Errorf("an unpinned download is bounded at %d bytes", (expect{}).limit())
	}
}

// A checksum this program does not check is refused before anything is asked
// for, never treated as "no checksum".
func TestAnUnknownChecksumIsRefusedBeforeFetching(t *testing.T) {
	server, asked := pinnedServer(t, []byte("x"), true)

	err := downloadVia(server.Client(), server.URL+"/x", filepath.Join(t.TempDir(), "x"),
		expect{algorithm: "md5", sum: "00"}, io.Discard)
	if err == nil {
		t.Fatal("a download checked with md5 went ahead")
	}

	if n := asked.Load(); n != 0 {
		t.Errorf("%d requests were made", n)
	}
}

/*
 * A part already on the machine is kept only when it is the published file.
 *
 * Somebody whose voices came from a branch before 1.0 is not sent 190MB again
 * when the bytes are the same, and is not left with them when they are not.
 */
func TestAPartAlreadyHereIsKeptOnlyWhenItIsThePublishedOne(t *testing.T) {
	published := []byte("the published model")
	dir := t.TempDir()
	into := filepath.Join(dir, "ggml-small.bin")

	// Nowhere anything can be fetched from: reaching the network fails.
	p := Pinned{Name: "the model", URL: "https://nowhere.invalid/model", Sum: sha(published),
		Algorithm: "sha256", Size: int64(len(published))}

	os.WriteFile(into, published, 0o644)

	if err := fetchUnlessHere(p, into, io.Discard); err != nil {
		t.Errorf("the published file already here was fetched again: %v", err)
	}

	os.WriteFile(into, []byte("a model from a branch"), 0o644)

	if err := fetchUnlessHere(p, into, io.Discard); err == nil {
		t.Error("a file that is not the published one was kept")
	}

	// And until a download that is the published file arrives, what is there
	// stays (the program still works with it); nothing half-written is added.
	if got, _ := os.ReadFile(into); string(got) != "a model from a branch" {
		t.Errorf("the file there became %q", got)
	}

	if parts := partFiles(t, dir); len(parts) > 0 {
		t.Errorf("left %v", parts)
	}
}

// The archive that is unpacked and run is fetched into a file of its own,
// never a fixed name in /tmp somebody else could have made first.
func TestTemporaryDownloadsHaveNamesOfTheirOwn(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	a, err := tempFile("pn-scripts-assistant-piper-*.tgz")
	if err != nil {
		t.Fatal(err)
	}

	b, err := tempFile("pn-scripts-assistant-piper-*.tgz")
	if err != nil {
		t.Fatal(err)
	}

	if a == b || filepath.Base(a) == "pn-scripts-assistant-piper.tgz" {
		t.Errorf("two downloads got %s and %s", a, b)
	}

	info, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("the temporary file is readable by others: %v", info.Mode())
	}

	// And it is still what ClearHalfFinished sweeps after a crash.
	if ok, _ := filepath.Match("pn-scripts-assistant-*", filepath.Base(a)); !ok {
		t.Errorf("%s is not swept after an interruption", a)
	}
}

/*
 * whisper.cpp is built only from the pinned commit.
 *
 * Each case serves a repository from this machine, laid out like the real
 * one: a tag on a commit, and a main branch that has moved past it.
 */

// gitRepo makes a repository with a commit tagged tag, then another commit on
// main after it, and returns its address and the tagged commit.
func gitRepo(t *testing.T, tag string) (string, string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	// Nothing of this machine's own git settings, and an author of its own.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")

	dir := t.TempDir()

	git := func(args ...string) string {
		t.Helper()

		cmd := exec.Command("git", args...)
		cmd.Dir = dir

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	git("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("release\n"), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "release")
	git("tag", "-a", tag, "-m", tag)

	tagged := git("rev-parse", "HEAD")

	os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("whatever main holds today\n"), 0o644)
	git("commit", "-q", "-am", "later")

	return "file://" + dir, tagged
}

func pinWhisper(t *testing.T, repo, tag, commit string) {
	t.Helper()

	old := whisperSource
	whisperSource.repo, whisperSource.tag, whisperSource.commit = repo, tag, commit

	t.Cleanup(func() { whisperSource = old })
}

func TestWhisperIsFetchedAtThePinnedCommit(t *testing.T) {
	repo, tagged := gitRepo(t, "v9.9.9")
	pinWhisper(t, repo, "v9.9.9", tagged)

	src := filepath.Join(t.TempDir(), "whisper.cpp")

	if err := fetchWhisperSource(io.Discard, src); err != nil {
		t.Fatal(err)
	}

	if at, _ := headOf(src); at != tagged {
		t.Errorf("fetched %s, want the pinned %s", at, tagged)
	}

	if got, _ := os.ReadFile(filepath.Join(src, "CMakeLists.txt")); string(got) != "release\n" {
		t.Errorf("the tree is %q, not the release", got)
	}
}

// A tag that has been moved to another commit is refused, and nothing is left
// for the next attempt to build.
func TestAMovedWhisperTagIsRefused(t *testing.T) {
	repo, _ := gitRepo(t, "v9.9.9")
	pinWhisper(t, repo, "v9.9.9", strings.Repeat("ab", 20))

	src := filepath.Join(t.TempDir(), "whisper.cpp")

	err := fetchWhisperSource(io.Discard, src)
	if err == nil || !strings.Contains(err.Error(), "nothing was built") {
		t.Fatalf("a tag on another commit gave %v", err)
	}

	if _, err := os.Stat(src); err == nil {
		t.Error("the refused copy was left to be built")
	}
}

// A copy from before 1.0, which followed main, is moved to the pinned commit
// in place, and keeps the speech model downloaded into it.
func TestAnOlderWhisperCopyIsMovedToThePinnedCommit(t *testing.T) {
	repo, tagged := gitRepo(t, "v9.9.9")

	src := filepath.Join(t.TempDir(), "whisper.cpp")

	if out, err := exec.Command("git", "clone", "-q", "--depth", "1", repo, src).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	os.MkdirAll(filepath.Join(src, "models"), 0o755)
	os.WriteFile(filepath.Join(src, "models", "ggml-small.bin"), []byte("model"), 0o644)

	if at, _ := headOf(src); at == tagged {
		t.Fatal("the older copy is already at the release")
	}

	pinWhisper(t, repo, "v9.9.9", tagged)

	if err := fetchWhisperSource(io.Discard, src); err != nil {
		t.Fatal(err)
	}

	if at, _ := headOf(src); at != tagged {
		t.Errorf("the copy is at %s, want the pinned %s", at, tagged)
	}

	if got, _ := os.ReadFile(filepath.Join(src, "models", "ggml-small.bin")); string(got) != "model" {
		t.Error("the speech model in the copy was lost")
	}
}

// A folder of somebody's own where the copy would go is left exactly as it is.
func TestAWhisperFolderThatIsNotOursIsLeftAlone(t *testing.T) {
	repo, tagged := gitRepo(t, "v9.9.9")
	pinWhisper(t, repo, "v9.9.9", tagged)

	src := filepath.Join(t.TempDir(), "whisper.cpp")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("mine"), 0o644)

	if err := fetchWhisperSource(io.Discard, src); err == nil || !strings.Contains(err.Error(), "nothing was built") {
		t.Fatalf("a folder that is not a whisper.cpp copy gave %v", err)
	}

	if got, _ := os.ReadFile(filepath.Join(src, "notes.txt")); string(got) != "mine" {
		t.Error("somebody's own folder was changed")
	}
}
