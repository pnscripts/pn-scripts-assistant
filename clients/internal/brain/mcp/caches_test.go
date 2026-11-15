package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * A server's runtime finds its cache folder there the first time.
 *
 * Found using the real window before 1.1.0: on a home where npm had never
 * run, an approved "npx -y ..." server would not start, and said only that
 * npm could not write its log folder. The server may write nowhere in the
 * home but its runtime's caches, and a folder that does not exist yet
 * cannot be opened to it, so npm could not make its own. The folders its
 * runtime keeps are made before it starts; nothing is made for a program
 * that has no use for them.
 */
func TestTheRuntimesCacheFoldersAreThereBeforeAServerStarts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bin := t.TempDir()

	for _, name := range []string{"npx", "uvx", "plain"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	there := func(parts ...string) bool {
		info, err := os.Stat(filepath.Join(append([]string{home}, parts...)...))

		return err == nil && info.IsDir()
	}

	start := func(name string) {
		t.Helper()

		s, err := StartStdio([]string{filepath.Join(bin, name)}, nil, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		s.Close()
	}

	start("plain")

	if there(".npm") || there(".cache") || there(".local", "share", "uv") {
		t.Fatal("folders were made in the home for a program that keeps no cache there")
	}

	start("npx")

	if !there(".npm") || !there(".cache") {
		t.Error("npm's folders were not there for a server started with npx")
	}

	if there(".local", "share", "uv") {
		t.Error("uv's folder was made for a server started with npx")
	}

	start("uvx")

	if !there(".local", "share", "uv") {
		t.Error("uv's folder was not there for a server started with uvx")
	}

	// Its own, and nobody else's to read.
	if info, err := os.Stat(filepath.Join(home, ".npm")); err == nil && info.Mode().Perm() != 0o700 {
		t.Errorf("the folder made is open to others: %v", info.Mode().Perm())
	}
}
