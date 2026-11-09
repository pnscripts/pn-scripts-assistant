package preflight

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// tarEntry is one header of a hand-built archive, in the order given.
type tarEntry struct {
	name, link, body string
	kind             byte
}

func tarOf(t *testing.T, entries []tarEntry) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	for _, e := range entries {
		h := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: e.kind, Mode: 0o755}
		if e.kind == tar.TypeReg {
			h.Mode = 0o644
			h.Size = int64(len(e.body))
		}

		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}

		if e.kind == tar.TypeReg {
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return &buf
}

/*
 * A link in an archive cannot carry a later file outside the destination.
 *
 * Checking each name is not enough: an entry can be a link first and a file
 * written through it next. An absolute link target joined to a folder looks
 * inside on paper, and a link to ".." from a folder that is itself a link to
 * "." lands one level above the destination. The digest is checked before
 * anything is unpacked, so only a pinned archive that is itself hostile could
 * do this; the unpacker still refuses, because it is the last line. (An
 * absolute link is simply not made, so the file after it lands inside.)
 */
func TestALinkInAnArchiveCannotCarryAFileOutside(t *testing.T) {
	cases := map[string][]tarEntry{
		"absolute link": {
			{name: "bin/", kind: tar.TypeDir},
			{name: "bin/evil", link: "OUTSIDE", kind: tar.TypeSymlink},
			{name: "bin/evil", body: "PWNED", kind: tar.TypeReg},
		},
		"link through a link": {
			{name: "x", link: ".", kind: tar.TypeSymlink},
			{name: "x/y", link: "..", kind: tar.TypeSymlink},
			{name: "x/y/escaped.txt", body: "PWNED", kind: tar.TypeReg},
		},
		"folder through a link": {
			{name: "x", link: ".", kind: tar.TypeSymlink},
			{name: "x/y", link: "..", kind: tar.TypeSymlink},
			{name: "x/y/made/", kind: tar.TypeDir},
			{name: "x/y/made/escaped.txt", body: "PWNED", kind: tar.TypeReg},
		},
	}

	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			into := filepath.Join(dir, "into")
			outside := filepath.Join(dir, "outside.txt")

			for i := range entries {
				if entries[i].link == "OUTSIDE" {
					entries[i].link = outside
				}
			}

			err := extractTar(tarOf(t, entries), into, 0, io.Discard)
			if err == nil && name != "absolute link" {
				t.Error("an archive that writes through a link to outside was accepted")
			}

			for _, escaped := range []string{
				outside,
				filepath.Join(dir, "escaped.txt"),
				filepath.Join(dir, "made"),
			} {
				if _, err := os.Lstat(escaped); err == nil {
					t.Errorf("%s was written outside the destination", escaped)
				}
			}
		})
	}
}

// The links a real release carries (a library and its versioned name) still work.
func TestALinkInsideTheArchiveStillWorks(t *testing.T) {
	into := filepath.Join(t.TempDir(), "into")

	err := extractTar(tarOf(t, []tarEntry{
		{name: "lib/", kind: tar.TypeDir},
		{name: "lib/libx.so.1.2", body: "binary", kind: tar.TypeReg},
		{name: "lib/libx.so.1", link: "libx.so.1.2", kind: tar.TypeSymlink},
		{name: "bin/", kind: tar.TypeDir},
		{name: "bin/lib", link: "../lib", kind: tar.TypeSymlink},
	}), into, 0, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	for _, through := range []string{"lib/libx.so.1", "bin/lib/libx.so.1.2"} {
		body, err := os.ReadFile(filepath.Join(into, through))
		if err != nil || string(body) != "binary" {
			t.Errorf("%s: %q, %v", through, body, err)
		}
	}
}

// A zip unpacks into a folder that does not exist yet, and refuses ../ names.
func TestAZipUnpacksIntoANewFolderAndStaysInside(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "release.zip")

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for name, body := range map[string]string{"tool-1.0/bin/tool": "binary", "tool-1.0/README": "hello"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	into := filepath.Join(dir, "new", "into")
	if err := unpackZip(archive, into, 1, io.Discard); err != nil {
		t.Fatal(err)
	}

	if body, err := os.ReadFile(filepath.Join(into, "bin", "tool")); err != nil || string(body) != "binary" {
		t.Errorf("bin/tool: %q, %v", body, err)
	}

	buf.Reset()
	zw = zip.NewWriter(&buf)

	w, err := zw.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}

	_, _ = io.WriteString(w, "PWNED")
	_ = zw.Close()

	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := unpackZip(archive, into, 0, io.Discard); err == nil {
		t.Error("a zip naming a path outside the destination was accepted")
	}

	if _, err := os.Stat(filepath.Join(dir, "new", "escaped.txt")); err == nil {
		t.Error("the zip wrote outside the destination")
	}
}
