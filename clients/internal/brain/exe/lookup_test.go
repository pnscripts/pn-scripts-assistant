package exe

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * A program in a place the desktop session's PATH does not mention.
 *
 * This is the whole reason the package exists. Launched from a terminal the
 * brain heard perfectly; launched from the applications menu it reported
 * "listening unavailable", because gnome-shell starts it with a short standard
 * PATH that has no ~/.local/bin on it — which is exactly where whisper.cpp
 * puts itself when it is built by hand.
 */
func TestFindingSomethingThatIsNotOnPath(t *testing.T) {
	home := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("PATH", "/nowhere")

	bin := filepath.Join(home, ".local", "bin")

	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}

	tool := filepath.Join(bin, "whisper-cli")

	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, ok := Look("whisper-cli")
	if !ok {
		t.Fatal("did not find a program sitting in ~/.local/bin")
	}

	if found != tool {
		t.Errorf("found %q, want %q", found, tool)
	}

	// The full path is returned, not the bare name: whatever runs it inherits
	// the same short PATH and would fail the same way.
	if !filepath.IsAbs(found) {
		t.Errorf("%q is not a path something else can run", found)
	}
}

// Something genuinely absent is still absent.
func TestNotFindingWhatIsNotThere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "/nowhere")

	if path, ok := Look("definitely-not-installed-anywhere"); ok {
		t.Errorf("found something that does not exist: %q", path)
	}
}

// A directory with the right name is not a program.
func TestADirectoryIsNotAProgram(t *testing.T) {
	home := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("PATH", "/nowhere")

	if err := os.MkdirAll(filepath.Join(home, ".local", "bin", "piper"), 0o755); err != nil {
		t.Fatal(err)
	}

	if path, ok := Look("piper"); ok {
		t.Errorf("took a directory for a program: %q", path)
	}
}
