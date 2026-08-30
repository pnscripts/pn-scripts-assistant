package preflight

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * An archive that tries to write outside where it was told to.
 *
 * An entry can be named ../../.bashrc, and an extractor that joins the paths
 * without checking writes it — the oldest bug in the format, still found in new
 * code every year. Everything here goes into a directory in somebody's home,
 * unattended, because they pressed a button that said "install".
 */
func TestAnArchiveCannotEscapeItsDirectory(t *testing.T) {
	dir := t.TempDir()
	into := filepath.Join(dir, "into")

	archive := filepath.Join(dir, "bad.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"../escaped.txt":       "should never be written",
		"nested/../../out.txt": "nor this",
		"fine.txt":             "this one is fine",
	})

	err := unpackTarGz(archive, into, 0, io.Discard)
	if err == nil {
		t.Error("an archive naming a path outside the destination was accepted")
	}

	for _, escaped := range []string{
		filepath.Join(dir, "escaped.txt"),
		filepath.Join(dir, "out.txt"),
	} {
		if _, err := os.Stat(escaped); err == nil {
			t.Errorf("%s was written outside the destination", escaped)
		}
	}
}

// A normal archive unpacks, with the release directory stripped off the front.
func TestUnpackingAReleaseArchive(t *testing.T) {
	dir := t.TempDir()
	into := filepath.Join(dir, "into")

	archive := filepath.Join(dir, "release.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"piper-1.2.0/piper":           "#!/bin/sh\n",
		"piper-1.2.0/lib/libpiper.so": "binary",
	})

	if err := unpackTarGz(archive, into, 1, io.Discard); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"piper", filepath.Join("lib", "libpiper.so")} {
		if _, err := os.Stat(filepath.Join(into, want)); err != nil {
			t.Errorf("%s did not come out of the archive: %v", want, err)
		}
	}

	// And the version directory is not still wrapped around everything.
	if _, err := os.Stat(filepath.Join(into, "piper-1.2.0")); err == nil {
		t.Error("the release directory was not stripped")
	}
}

/*
 * Only https, and only to a project's own release host.
 *
 * The usual instruction for each of these is to pipe a remote script into a
 * shell. This does not do that and will not, so the one thing it does do —
 * fetch an archive — has to be narrow enough to be worth trusting.
 */
func TestWhereItWillAndWillNotFetchFrom(t *testing.T) {
	for _, ok := range []string{
		"https://ollama.com/download/ollama-linux-amd64.tgz",
		"https://github.com/ggerganov/whisper.cpp/archive/refs/tags/v1.5.4.tar.gz",
		"https://huggingface.co/rhasspy/piper-voices/resolve/main/en_GB-alba-medium.onnx",

		// Where a release download actually ends up. github.com answers with a
		// redirect to a signed URL on a content host, and refusing that meant
		// the installer refused its own download halfway through.
		"https://release-assets.githubusercontent.com/github-production-release-asset/587499842/x?sig=y",
		"https://objects.githubusercontent.com/thing",
		"https://cdn-lfs-us-1.hf.co/repos/model.onnx",
	} {
		if err := allowed(ok); err != nil {
			t.Errorf("refused a project's own release: %s (%v)", ok, err)
		}
	}

	for _, no := range []string{
		"http://ollama.com/download/ollama-linux-amd64.tgz",
		"https://example.com/totally-fine.tgz",
		"file:///etc/passwd",
		"https://github.com.evil.test/thing.tgz",
		"https://notgithubusercontent.com/thing.tgz",
		"https://evil.test/githubusercontent.com",
	} {
		if err := allowed(no); err == nil {
			t.Errorf("would have fetched from %s", no)
		}
	}
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}

		if _, err := io.WriteString(tw, body); err != nil {
			t.Fatal(err)
		}
	}

	tw.Close()
	gz.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

var _ = strings.TrimSpace

/*
 * Everything a new machine needs must be installable without a terminal.
 *
 * This is the whole point of the setup page. Somebody who has never heard of a
 * language model, on a machine they bought last week, cannot be told to build
 * whisper.cpp from source — and until this was written, three of the things
 * they need had no button at all: Ollama, a voice worth listening to, and the
 * recogniser. All three said "do it yourself" to precisely the person who
 * cannot.
 */
func TestEverythingNeededCanBeInstalledFromTheApp(t *testing.T) {
	for _, r := range Requirements() {
		if !r.Installable() {
			t.Errorf("%s can only be installed by hand: %s", r.Name, r.ManualHint)
		}
	}
}

// And nothing claims to be installable without saying how.
func TestNothingClaimsAnInstallItCannotDo(t *testing.T) {
	for _, r := range Requirements() {
		if r.InstallFunc == nil && r.InstallCmd == nil && r.Installable() {
			t.Errorf("%s says it can be installed and has no way to", r.Name)
		}

		if r.InstallFunc != nil && r.InstallCmd != nil {
			t.Errorf("%s has two ways to install it; which one runs is a coin toss", r.Name)
		}

		// A person has to be told what they are agreeing to.
		if r.Why == "" || r.Consequence == "" {
			t.Errorf("%s does not say what it is for or what breaks without it", r.Name)
		}
	}
}
