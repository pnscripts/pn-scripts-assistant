package preflight

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/*
 * Installing the things that have no package.
 *
 * Ollama, piper and whisper.cpp are all distributed as an archive from their
 * own project, and the usual instruction for each is to pipe a remote script
 * into a shell. This does not do that, and will not: a script fetched and run
 * unread can do anything at all, and an application that teaches its owner to
 * accept that has taught them something worse than it fixed.
 *
 * What happens instead is narrow and inspectable. A file is downloaded over
 * HTTPS from the project's own release host, its size is checked, and the
 * archive is unpacked into the user's own directory. Nothing downloaded is
 * executed during installation, nothing needs a password, and nothing outside
 * the home directory is touched.
 */

// allowedHosts are the projects whose own releases may be fetched.
//
// A list, so that a redirect to somewhere else fails rather than being
// followed. Everything here is the upstream project for a thing the owner has
// asked to install.
var allowedHosts = map[string]bool{
	"ollama.com":     true,
	"github.com":     true,
	"huggingface.co": true,
}

/*
 * allowedSuffixes are the content hosts those projects redirect to.
 *
 * A release download from github.com does not come from github.com: it answers
 * with a redirect to a signed URL on release-assets.githubusercontent.com, and
 * which subdomain that is has changed at least twice. Matching the parent
 * domain is safe in a way that matching a substring would not be — nobody else
 * can be given a name under githubusercontent.com — and it is checked with a
 * leading dot so that a host merely ending in those letters does not qualify.
 *
 * Found by running the installer rather than by reading it: the first attempt
 * refused its own download halfway through.
 */
var allowedSuffixes = []string{
	".githubusercontent.com",
	".huggingface.co",
	".hf.co",
}

// mostBytes bounds a download, so a wrong URL cannot fill the disk.
const mostBytes = 3 << 30 // 3GB

/*
 * download fetches one file, reporting progress as it goes.
 *
 * The progress matters more than it looks. These are hundreds of megabytes on
 * a home connection, and a button that does nothing visible for four minutes
 * is a button somebody presses again.
 */
func download(from, to string, w io.Writer) error {
	// Where it is allowed to fetch from, decided before anything is opened.
	// downloadVia does the work and does not repeat this check, which is why
	// nothing outside this file may call it with an address from elsewhere.
	if err := allowed(from); err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 2 * time.Hour,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Redirects are normal here — release hosts hand off to a CDN —
			// but each hop is checked rather than trusted because the first
			// one was.
			return allowed(req.URL.String())
		},
	}

	return downloadVia(client, from, to, w)
}

/*
 * downloadVia is the fetching, separated from the deciding.
 *
 * Split out so the part that has been wrong — what is left on disk when a
 * download stops early — can be tested against a server that stops early. It
 * carries no opinion about which addresses are allowed; download does that
 * above, before this is reached.
 */
func downloadVia(client *http.Client, from, to string, w io.Writer) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	res, err := client.Get(from)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", from, err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", from, res.Status)
	}

	/*
	 * Written beside the destination and moved into place at the end.
	 *
	 * It used to write straight to the final path, which turns "somebody
	 * closed the window" into a permanent broken install: a 488MB speech model
	 * interrupted at 60MB leaves a 60MB file with the right name, and every
	 * check in this program that asks "is it installed" looks at the name and
	 * the size and says yes. Whisper then fails to load it, with an error
	 * about the file format, on a machine where nothing appears to be wrong.
	 *
	 * A part file is nothing anybody mistakes for the real thing, and it is
	 * removed on the way out however this ends. The rename is the only moment
	 * the install becomes true, and a rename is atomic.
	 */
	part := to + ".part"

	file, err := os.Create(part)
	if err != nil {
		return err
	}

	// Removed unless the rename below has already taken it. Interruptions get
	// here through the process dying rather than through this defer, which is
	// why the name matters as much as the cleanup.
	defer func() {
		file.Close()
		os.Remove(part)
	}()

	fmt.Fprintf(w, "Downloading %s\n", filepath.Base(from))

	counted := &progress{out: w, total: res.ContentLength, every: 4 * time.Second}

	if _, err := io.Copy(file, io.TeeReader(io.LimitReader(res.Body, mostBytes), counted)); err != nil {
		return fmt.Errorf("downloading %s: %w", from, err)
	}

	/*
	 * All of it, not most of it.
	 *
	 * A copy that ends without an error is not a complete download: a
	 * connection dropped cleanly mid-file reads as end of stream. When the
	 * server said how big it is, that is the only check worth making, and it
	 * is the difference between a file that fails now and one that fails in
	 * three weeks with no explanation.
	 */
	if res.ContentLength > 0 && counted.done != res.ContentLength {
		return fmt.Errorf("%s stopped early: %s of %s",
			filepath.Base(from), readable(counted.done), readable(res.ContentLength))
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(to), err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(to), err)
	}

	if err := os.Rename(part, to); err != nil {
		return fmt.Errorf("putting %s in place: %w", filepath.Base(to), err)
	}

	fmt.Fprintf(w, "Downloaded %s\n", readable(counted.done))

	return nil
}

// allowed refuses anything that is not HTTPS to a project's own release host.
func allowed(raw string) error {
	at, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q is not an address", raw)
	}

	if at.Scheme != "https" {
		return fmt.Errorf("refusing to fetch over %s; only https", at.Scheme)
	}

	host := at.Hostname()

	if allowedHosts[host] {
		return nil
	}

	for _, suffix := range allowedSuffixes {
		if strings.HasSuffix(host, suffix) {
			return nil
		}
	}

	return fmt.Errorf("refusing to fetch from %s: not a project this installs from", host)
}

// progress reports how far a download has got.
type progress struct {
	out   io.Writer
	total int64
	done  int64
	every time.Duration
	last  time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))

	if time.Since(p.last) >= p.every {
		p.last = time.Now()

		if p.total > 0 {
			fmt.Fprintf(p.out, "  %s of %s\n", readable(p.done), readable(p.total))
		} else {
			fmt.Fprintf(p.out, "  %s\n", readable(p.done))
		}
	}

	return len(b), nil
}

func readable(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%dKB", n/1024)
	}
}

/*
 * unpackTarGz extracts an archive into a directory.
 *
 * Every entry's path is checked against the destination before anything is
 * written. An archive can name a file as ../../.bashrc, and an extractor that
 * simply joins the paths will cheerfully write it — the oldest bug in the
 * format, and the reason this is written out rather than shelled to tar.
 */
func unpackTarGz(archive, into string, strip int, w io.Writer) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}

	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s is not a gzip archive: %w", filepath.Base(archive), err)
	}

	defer gz.Close()

	return extractTar(gz, into, strip, w)
}

// unpackTar is the same for an archive that has already been decompressed —
// zstd, for instance, which is handed to the system's own program.
func unpackTar(archive, into string, strip int, w io.Writer) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}

	defer f.Close()

	return extractTar(f, into, strip, w)
}

// extractTar is the part that checks every path, whatever decompressed it.
func extractTar(from io.Reader, into string, strip int, w io.Writer) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}

	reader := tar.NewReader(from)
	files := 0

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		name := stripLeading(header.Name, strip)
		if name == "" {
			continue
		}

		target, err := safeJoin(into, name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(target, reader, header.FileInfo().Mode()); err != nil {
				return err
			}

			files++
		case tar.TypeSymlink:
			// Only within the destination, for the same reason as above.
			if _, err := safeJoin(into, filepath.Join(filepath.Dir(name), header.Linkname)); err != nil {
				continue
			}

			_ = os.Remove(target)
			_ = os.Symlink(header.Linkname, target)
		}
	}

	fmt.Fprintf(w, "Unpacked %d files into %s\n", files, into)

	return nil
}

// unpackZip is the same for the projects that ship a zip.
func unpackZip(archive, into string, strip int, w io.Writer) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}

	defer r.Close()

	files := 0

	for _, entry := range r.File {
		name := stripLeading(entry.Name, strip)
		if name == "" || strings.HasSuffix(entry.Name, "/") {
			continue
		}

		target, err := safeJoin(into, name)
		if err != nil {
			return err
		}

		body, err := entry.Open()
		if err != nil {
			return err
		}

		err = writeEntry(target, body, entry.Mode())

		body.Close()

		if err != nil {
			return err
		}

		files++
	}

	fmt.Fprintf(w, "Unpacked %d files into %s\n", files, into)

	return nil
}

func writeEntry(target string, from io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}

	defer out.Close()

	_, err = io.Copy(out, io.LimitReader(from, mostBytes))

	return err
}

/*
 * safeJoin refuses a path that would land outside the destination.
 *
 * An archive can name an entry ../../.bashrc, and an extractor that joins the
 * paths without checking writes it. This is the oldest bug in the format and it
 * is still found in new code every year.
 */
func safeJoin(into, name string) (string, error) {
	target := filepath.Join(into, name)

	rel, err := filepath.Rel(into, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q would write outside %s", name, into)
	}

	return target, nil
}

// stripLeading removes the first n path components, which is how these archives
// are shipped: everything inside one directory named after the release.
func stripLeading(name string, n int) string {
	name = filepath.Clean(strings.TrimPrefix(name, "./"))

	for i := 0; i < n; i++ {
		slash := strings.Index(name, "/")
		if slash < 0 {
			return ""
		}

		name = name[slash+1:]
	}

	return name
}

/*
 * ClearHalfFinished removes what an interrupted install left behind.
 *
 * Closing the window during a download used to be permanent: the part file did
 * not exist, so the truncated download wore the real name and every check
 * agreed it was installed. Part files fix that going forward and they
 * accumulate — one per interrupted attempt — so somebody who closed setup
 * three times has three of them and no idea what they are.
 *
 * Called at startup rather than at install time, because the moment worth
 * cleaning up is the one after the interruption rather than the one before the
 * next attempt: a person who never tries again should not be left carrying a
 * gigabyte of nothing.
 */
func ClearHalfFinished() (removed int, freed int64) {
	/*
	 * The downloaded archives first, which live in the temp directory.
	 *
	 * Ollama arrives as a 1.3GB compressed tar that is unpacked and deleted,
	 * and the deleting is a deferred call — so it happens on every ordinary
	 * path and on none of the ones that matter here. A process killed during
	 * an install, or a machine turned off, leaves two gigabytes in /tmp that
	 * nothing afterwards was looking for.
	 *
	 * By this program's own prefix and nothing else: a sweep of /tmp by
	 * pattern is how somebody else's work gets deleted.
	 */
	if archives, err := filepath.Glob(filepath.Join(os.TempDir(), "pn-brain-*")); err == nil {
		for _, at := range archives {
			info, err := os.Stat(at)
			if err != nil || info.IsDir() {
				continue
			}

			if os.RemoveAll(at) == nil {
				removed++
				freed += info.Size()
			}
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return removed, freed
	}

	// Where this program puts things it downloads, and nowhere else. A sweep
	// for "*.part" across a home directory would delete somebody's own files.
	for _, dir := range []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".local", "share", "piper"),

		// The voices land a directory deeper than piper itself, and three of
		// them are fetched in a row — so an interrupted install leaves its
		// half-written file here rather than in the directory above.
		filepath.Join(home, ".local", "share", "piper", "voices"),
		filepath.Join(home, ".local", "src", "whisper.cpp", "models"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".part") {
				continue
			}

			at := filepath.Join(dir, e.Name())

			info, err := e.Info()
			if err == nil {
				freed += info.Size()
			}

			if os.Remove(at) == nil {
				removed++
			}
		}
	}

	return removed, freed
}
