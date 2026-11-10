package config

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * The address is set under the program's name, and the old name still works.
 *
 * BRAIN_ADDR is from when the program was PN Brain (owner, 10.11.2026: names a
 * person types say what the program is called). PN_SCRIPTS_ASSISTANT_ADDR is
 * the one documented now; BRAIN_ADDR keeps working from the environment and
 * from brain.conf, because a setting somebody wrote down must not stop working
 * on an update.
 */
func TestTheAddressIsSetUnderTheProgramsName(t *testing.T) {
	t.Setenv(AddrEnv, "")
	t.Setenv("BRAIN_ADDR", "")

	root := t.TempDir()

	load := func() string {
		t.Helper()

		cfg, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}

		return cfg.Addr
	}

	if got := load(); got != Default().Addr {
		t.Errorf("with nothing set the address is %s", got)
	}

	// The old variable alone, as scripts written for 1.0 set it.
	t.Setenv("BRAIN_ADDR", "127.0.0.1:9101")
	if got := load(); got != "127.0.0.1:9101" {
		t.Errorf("BRAIN_ADDR is no longer read: %s", got)
	}

	// The new one wins when both are there.
	t.Setenv(AddrEnv, "127.0.0.1:9102")
	if got := load(); got != "127.0.0.1:9102" {
		t.Errorf("%s does not win over BRAIN_ADDR: %s", AddrEnv, got)
	}

	if AddrEnv != "PN_SCRIPTS_ASSISTANT_ADDR" {
		t.Errorf("the variable is called %s", AddrEnv)
	}
}

// A brain.conf written by an older version, with its BRAIN_ADDR line, is read
// as it was: the file keeps its key names.
func TestAnOldSettingsFileKeepsItsAddress(t *testing.T) {
	t.Setenv(AddrEnv, "")
	t.Setenv("BRAIN_ADDR", "")

	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "brain.conf"),
		[]byte("BRAIN_NAME=Alex\nBRAIN_ADDR=127.0.0.1:9103\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Addr != "127.0.0.1:9103" {
		t.Errorf("an old file's BRAIN_ADDR came back as %s", cfg.Addr)
	}

	// The environment still overrides the file, under the new name as well.
	t.Setenv(AddrEnv, "127.0.0.1:9104")

	if cfg, _ = Load(root); cfg.Addr != "127.0.0.1:9104" {
		t.Errorf("%s does not override the file: %s", AddrEnv, cfg.Addr)
	}
}
