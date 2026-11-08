package voiceprint

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/*
 * Fetching the two parts, from inside the program.
 *
 * Both are downloads rather than things compiled in: the model is thirty
 * megabytes and most people will never turn this on, and the runtime is a
 * shared library built for one processor and one operating system. Carrying
 * either in the binary would make every copy of the program bigger for a
 * feature that is off by default.
 */

// Where the parts come from, and how big they are, so the interface can say
// what it is about to download before it starts.
const (
	ModelFrom = "https://huggingface.co/csukuangfj/speaker-embedding-models/" +
		"resolve/main/3dspeaker_speech_campplus_sv_en_voxceleb_16k.onnx"

	/*
	 * The runtime release, which is not a free choice.
	 *
	 * The Go binding (github.com/yalue/onnxruntime_go) is compiled against one
	 * version of the runtime's C interface and asks the library for exactly
	 * that version when it starts: binding v1.38.0 carries ORT_API_VERSION
	 * 31, which is onnxruntime 1.31. A library from another release either
	 * refuses or, worse, answers with a table of functions laid out
	 * differently. TestTheRuntimeMatchesTheBinding reads the binding's header
	 * and fails the build when the two drift apart.
	 */
	RuntimeVersion = "1.31.0"
	RuntimeFrom    = "https://github.com/microsoft/onnxruntime/releases/download/v" +
		RuntimeVersion + "/onnxruntime-linux-x64-" + RuntimeVersion + ".tgz"

	/*
	 * RuntimeSHA256 is the archive above, byte for byte.
	 *
	 * Taken from the official release (the digest GitHub publishes for the
	 * asset) and confirmed by downloading it and hashing it, on 2026-11-08.
	 * The library ends up loaded into this program, so an archive that is not
	 * exactly this one is refused, whoever served it and however it got
	 * changed on the way.
	 */
	RuntimeSHA256 = "cc5c72baf5ae5c8238a6841f0897227be2d02826b9cf98eaf02fdefaeeb03a57"

	// RuntimeStamp sits beside the library and names the release it came
	// from. A library without one was installed by a copy of the program
	// that did not write it, which is to say an older release.
	RuntimeStamp = "libonnxruntime.version"

	// mostRuntimeBytes bounds the archive (eleven megabytes today), so a
	// server that never stops sending cannot fill the disk.
	mostRuntimeBytes = 256 << 20

	// AboutMegabytes is the pair, for saying so before asking.
	AboutMegabytes = 40
)

// Fetching is how long the whole download may take before it is given up on.
const Fetching = 20 * time.Minute

// Where the runtime is fetched from and what it must hash to. Variables only
// so the tests can serve their own archive from this machine; nothing else
// changes them.
var (
	runtimeFrom = RuntimeFrom
	runtimeSum  = RuntimeSHA256
)

// ErrRuntimeTampered is a runtime archive that is not the one released.
var ErrRuntimeTampered = errors.New("the downloaded runtime is not the published one")

/*
 * RuntimeCurrent reports whether the runtime on this machine is the release
 * this copy of the program was built to talk to.
 */
func RuntimeCurrent() bool {
	return runtimeCurrentIn(Parts())
}

func runtimeCurrentIn(into string) bool {
	if _, err := os.Stat(filepath.Join(into, RuntimeName)); err != nil {
		return false
	}

	stamp, err := os.ReadFile(filepath.Join(into, RuntimeStamp))

	return err == nil && strings.TrimSpace(string(stamp)) == RuntimeVersion
}

/*
 * RuntimeStale reports a runtime that is here but belongs to another release:
 * the program was updated and the library it downloaded earlier was not.
 */
func RuntimeStale() bool {
	into := Parts()

	_, err := os.Stat(filepath.Join(into, RuntimeName))

	return err == nil && !runtimeCurrentIn(into)
}

/*
 * Refresh replaces a runtime left behind by an older release.
 *
 * Only for somebody who already chose this feature: the model and a runtime
 * are both on the machine, so the download was agreed to once. Somebody who
 * never turned it on is never sent anything. Reports whether it replaced
 * anything.
 *
 * Until it has, Installed says no, so the old library is never loaded by a
 * program that would talk to it in the wrong language.
 */
func Refresh(ctx context.Context, say func(string)) (bool, error) {
	if say == nil {
		say = func(string) {}
	}

	into := Parts()

	if _, err := os.Stat(filepath.Join(into, ModelName)); err != nil {
		return false, nil
	}

	if !RuntimeStale() {
		return false, nil
	}

	ctx, stop := context.WithTimeout(ctx, Fetching)
	defer stop()

	say("Updating what tells voices apart to the version this copy needs (about 11MB)…")

	if err := runtime(ctx, into); err != nil {
		return false, err
	}

	say("It can tell voices apart again.")

	return true, nil
}

/*
 * Install downloads the model and the runtime.
 *
 * Reports progress through say, because this is forty megabytes on a home
 * connection and a button that goes quiet for two minutes is a button somebody
 * presses again.
 */
func Install(ctx context.Context, say func(string)) error {
	if say == nil {
		say = func(string) {}
	}

	into := Parts()

	if err := os.MkdirAll(into, 0o755); err != nil {
		return fmt.Errorf("making %s: %w", into, err)
	}

	ctx, stop := context.WithTimeout(ctx, Fetching)
	defer stop()

	if _, err := os.Stat(filepath.Join(into, ModelName)); err != nil {
		say("Downloading the model that tells voices apart (about 28MB)…")

		if err := download(ctx, ModelFrom, filepath.Join(into, ModelName)); err != nil {
			return err
		}
	}

	// Missing, or left by an older release: either way the matching one.
	if !runtimeCurrentIn(into) {
		say("Downloading what runs it (about 11MB)…")

		if err := runtime(ctx, into); err != nil {
			return err
		}
	}

	say("It can tell voices apart now.")

	return nil
}

// download fetches one file, writing beside the destination and renaming, so
// an interrupted download never looks like a finished one.
func download(ctx context.Context, from, to string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", from, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: the server said %s", from, res.Status)
	}

	working := to + ".part"

	file, err := os.Create(working)
	if err != nil {
		return err
	}

	if _, err := io.Copy(file, res.Body); err != nil {
		file.Close()
		os.Remove(working)

		return fmt.Errorf("writing %s: %w", to, err)
	}

	if err := file.Close(); err != nil {
		os.Remove(working)

		return err
	}

	return os.Rename(working, to)
}

/*
 * runtime fetches the release archive, proves it is the published one, and
 * unpacks the one shared library out of it.
 *
 * The whole archive is kept on disk and hashed before anything is taken out
 * of it: unpacking while it streams in would put a library on the disk before
 * the last byte, and so before the hash, was known.
 *
 * The archive carries headers, a second provider library and a cmake
 * directory, none of which this needs, so one file is taken out of it and the
 * rest is never written to disk.
 */
func runtime(ctx context.Context, into string) error {
	archivePath := filepath.Join(into, "onnxruntime.tgz.part")
	defer os.Remove(archivePath)

	if err := fetchVerified(ctx, runtimeFrom, runtimeSum, archivePath); err != nil {
		return err
	}

	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	unzipped, err := gzip.NewReader(archiveFile)
	if err != nil {
		return err
	}
	defer unzipped.Close()

	archive := tar.NewReader(unzipped)

	for {
		entry, err := archive.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		/*
		 * The versioned file, not the symbolic links beside it.
		 *
		 * The archive holds libonnxruntime.so and libonnxruntime.so.1 as links
		 * to libonnxruntime.so.<version>. Copying a link out of an archive
		 * gives a link pointing at a path that does not exist here, so the
		 * real file is taken and given the plain name.
		 */
		if entry.Typeflag != tar.TypeReg ||
			filepath.Base(entry.Name) != "libonnxruntime.so."+RuntimeVersion {
			continue
		}

		to := filepath.Join(into, RuntimeName)

		if err := writeAtomically(to, archive); err != nil {
			return err
		}

		// The stamp last: a crash before it leaves a library that reads as
		// stale and is fetched again, never a stamp vouching for a library
		// that is not there.
		return writeAtomically(filepath.Join(into, RuntimeStamp),
			strings.NewReader(RuntimeVersion+"\n"))
	}

	return fmt.Errorf("the runtime archive did not contain libonnxruntime.so.%s", RuntimeVersion)
}

// fetchVerified downloads from into to and fails, removing it, unless its
// SHA-256 is want.
func fetchVerified(ctx context.Context, from, want, to string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching the runtime: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching the runtime: the server said %s", res.Status)
	}

	file, err := os.Create(to)
	if err != nil {
		return err
	}

	hash := sha256.New()

	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(res.Body, mostRuntimeBytes+1))
	if cerr := file.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		os.Remove(to)

		return fmt.Errorf("fetching the runtime: %w", err)
	}

	if n > mostRuntimeBytes {
		os.Remove(to)

		return fmt.Errorf("%w: it is larger than %d megabytes", ErrRuntimeTampered, mostRuntimeBytes>>20)
	}

	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		os.Remove(to)

		return fmt.Errorf("%w: its SHA-256 is %s, the release's is %s; nothing was installed",
			ErrRuntimeTampered, got, want)
	}

	return nil
}

// writeAtomically writes beside the destination and renames, so an
// interrupted write never looks like a finished one.
func writeAtomically(to string, from io.Reader) error {
	working := to + ".part"

	file, err := os.Create(working)
	if err != nil {
		return err
	}

	if _, err := io.Copy(file, from); err != nil {
		file.Close()
		os.Remove(working)

		return err
	}

	if err := file.Close(); err != nil {
		os.Remove(working)

		return err
	}

	return os.Rename(working, to)
}
