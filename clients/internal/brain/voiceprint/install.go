package voiceprint

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

	RuntimeVersion = "1.29.0"
	RuntimeFrom    = "https://github.com/microsoft/onnxruntime/releases/download/v" +
		RuntimeVersion + "/onnxruntime-linux-x64-" + RuntimeVersion + ".tgz"

	// AboutMegabytes is the pair, for saying so before asking.
	AboutMegabytes = 40
)

// Fetching is how long the whole download may take before it is given up on.
const Fetching = 20 * time.Minute

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

	if _, err := os.Stat(filepath.Join(into, RuntimeName)); err != nil {
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
 * runtime unpacks the one shared library out of the release archive.
 *
 * The archive carries headers, a second provider library and a cmake
 * directory, none of which this needs — so one file is taken out of it and the
 * rest is never written to disk.
 */
func runtime(ctx context.Context, into string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, RuntimeFrom, nil)
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

	unzipped, err := gzip.NewReader(res.Body)
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
		 * to libonnxruntime.so.1.29.0. Copying a link out of an archive gives
		 * a link pointing at a path that does not exist here, so the real file
		 * is taken and given the plain name.
		 */
		name := filepath.Base(entry.Name)

		if entry.Typeflag != tar.TypeReg ||
			!strings.HasPrefix(name, "libonnxruntime.so."+RuntimeVersion[:1]) ||
			strings.Contains(name, "providers") {
			continue
		}

		to := filepath.Join(into, RuntimeName)
		working := to + ".part"

		file, err := os.Create(working)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, archive); err != nil {
			file.Close()
			os.Remove(working)

			return err
		}

		file.Close()

		return os.Rename(working, to)
	}

	return fmt.Errorf("the runtime archive did not contain %s", RuntimeName)
}
