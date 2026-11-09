//go:build cgo

package voiceprint

import (
	"os"
	"path/filepath"
	"testing"
)

// A runtime from another release is not "installed": loading it would talk
// to the library through a table of functions it does not have.
func TestAnOlderRuntimeDoesNotCountAsInstalled(t *testing.T) {
	into := machine(t)

	os.WriteFile(filepath.Join(into, ModelName), []byte("model"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), []byte("old"), 0o755)

	if Installed() {
		t.Error("an unstamped runtime counts as installed")
	}

	os.WriteFile(filepath.Join(into, RuntimeStamp), []byte("1.29.0\n"), 0o644)

	if Installed() {
		t.Error("a 1.29.0 runtime counts as installed")
	}

	os.WriteFile(filepath.Join(into, RuntimeStamp), []byte(RuntimeVersion+"\n"), 0o644)

	if !Installed() {
		t.Error("the matching runtime with its model does not count as installed")
	}

	os.Remove(filepath.Join(into, ModelName))

	if Installed() {
		t.Error("a runtime without the model counts as installed")
	}
}

// Asking before the parts arrive must not keep refusing after they have.
func TestOpenDoesNotRememberAMissingInstall(t *testing.T) {
	machine(t)

	if _, err := open(); err == nil {
		t.Fatal("open succeeded with nothing installed")
	}

	loaded.mu.Lock()
	started := loaded.started
	loaded.mu.Unlock()

	if started {
		t.Error("a refusal for missing parts was remembered for the life of the program")
	}
}

// A model that is not the published file does not count as installed, so the
// page offers to fetch it and nothing loads it.
func TestAModelThatIsNotThePublishedOneIsNotInstalled(t *testing.T) {
	into := machine(t)

	os.WriteFile(filepath.Join(into, ModelName), []byte("whatever main held in 0.13"), 0o644)
	os.WriteFile(filepath.Join(into, RuntimeName), []byte("lib"), 0o755)
	os.WriteFile(filepath.Join(into, RuntimeStamp), []byte(RuntimeVersion+"\n"), 0o644)

	if Installed() {
		t.Error("a model that is not the published file counts as installed")
	}
}
