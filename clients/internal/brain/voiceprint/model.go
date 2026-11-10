//go:build cgo

package voiceprint

import (
	"fmt"
	"os"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/paths"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

/*
 * The model that turns a voice into numbers.
 *
 * A speaker model (none is offered since 1.0.1, see Offered): a few seconds
 * of audio in, five hundred and twelve numbers out, describing who is speaking rather than what they said.
 * Two recordings of one person land close together; two people do not. That is
 * the whole mechanism, and everything above it is arithmetic on the result.
 *
 * Loaded once and kept, because loading is the expensive part and a voice has
 * to be judged inside the pause between somebody finishing a sentence and the
 * brain deciding whether it was addressed.
 */

var loaded struct {
	mu      sync.Mutex
	session *ort.DynamicAdvancedSession
	started bool
	err     error
}

/*
 * Two different places, because these are two different kinds of thing.
 *
 * The model and the runtime are parts of this machine, like the recogniser and
 * the voice: a shared library built for this processor, and a file that would
 * be downloaded again on any other computer. They live where the other machine
 * parts live.
 *
 * The voiceprint is not. It is the one thing here that belongs to a person
 * rather than to a computer, so it lives in the brain's own folder and travels
 * with it — plug the drive into another machine and it still knows whose voice
 * to listen for, without being taught again.
 */
func Parts() string {
	here := filepath.Join(paths.MachineFolder(), FolderName)

	// Installed before the rename, under the old folder, and not downloaded
	// again for the sake of a name.
	if _, err := os.Stat(here); err != nil {
		if legacy := paths.LegacyHomeRoot(); legacy != "" {
			if _, err := os.Stat(filepath.Join(legacy, FolderName)); err == nil {
				return filepath.Join(legacy, FolderName)
			}
		}
	}

	return here
}

// Folder is where this brain keeps the voice it knows.
func Folder(root string) string { return filepath.Join(root, FolderName) }

/*
 * Installed reports whether the model and the runtime are both on this
 * machine, and the runtime is the release this copy was built for.
 *
 * A runtime from another release counts as missing rather than as present:
 * loading it would mean talking to a library through a table of functions it
 * does not have. Refresh replaces it; until then voices are not told apart,
 * which is a feature briefly absent rather than one quietly wrong.
 */
func Installed() bool {
	// A model an older release left is not loaded while none is offered.
	if !Offered() {
		return false
	}

	into := Parts()

	if _, err := os.Stat(filepath.Join(into, ModelName)); err != nil {
		return false
	}

	// The runtime first: it is a stamp to read, and a stale one means not
	// installed whatever the model is. Then the model, which is hashed once
	// (modelVerifiedIn): one that is not the published file is never loaded.
	return RuntimeCurrent() && modelVerifiedIn(into)
}

/*
 * open loads the runtime and the model, once.
 *
 * The runtime is a shared library rather than something compiled in, so it is
 * found at run time and its absence is a fact about the machine rather than a
 * failure to build. A brain without it simply cannot tell voices apart, which
 * is a feature it does not have rather than a fault.
 */
func open() (*ort.DynamicAdvancedSession, error) {
	loaded.mu.Lock()
	defer loaded.mu.Unlock()

	if loaded.started {
		return loaded.session, loaded.err
	}

	// Not remembered: the parts can arrive while the program runs (installed
	// from the page, or refreshed after an update), and a refusal kept from
	// before would outlive the reason for it.
	if !Installed() {
		return nil, fmt.Errorf("no voiceprint model on this machine; nothing to tell voices apart with")
	}

	loaded.started = true

	ort.SetSharedLibraryPath(filepath.Join(Parts(), RuntimeName))

	if err := ort.InitializeEnvironment(); err != nil {
		loaded.err = fmt.Errorf("starting the voiceprint runtime: %w", err)

		return nil, loaded.err
	}

	session, err := ort.NewDynamicAdvancedSession(
		filepath.Join(Parts(), ModelName),
		[]string{"x"}, []string{"embedding"}, nil)
	if err != nil {
		loaded.err = fmt.Errorf("loading the voiceprint model: %w", err)

		return nil, loaded.err
	}

	loaded.session = session

	return session, nil
}

/*
 * Of returns the voiceprint of a recording.
 *
 * samples are mono at sixteen kilohertz in the scale a sixteen-bit recording
 * arrives in — see Fbank for why the scale matters.
 */
func Of(samples []float64) ([]float32, error) {
	if seconds := float64(len(samples)) / SampleRate; seconds < LeastSeconds {
		return nil, fmt.Errorf(
			"only %.1f seconds of sound; too little to tell whose voice it is", seconds)
	}

	session, err := open()
	if err != nil {
		return nil, err
	}

	features := Fbank(samples)

	if len(features) == 0 {
		return nil, fmt.Errorf("nothing to measure in that recording")
	}

	MeanNormalise(features)

	flat := make([]float32, 0, len(features)*Bins)

	for _, row := range features {
		for _, v := range row {
			flat = append(flat, float32(v))
		}
	}

	in, err := ort.NewTensor(ort.NewShape(1, int64(len(features)), Bins), flat)
	if err != nil {
		return nil, err
	}
	defer in.Destroy()

	out, err := ort.NewEmptyTensor[float32](ort.NewShape(1, Dimensions))
	if err != nil {
		return nil, err
	}
	defer out.Destroy()

	if err := session.Run([]ort.Value{in}, []ort.Value{out}); err != nil {
		return nil, fmt.Errorf("measuring the voice: %w", err)
	}

	// Copied out: the tensor's memory belongs to the runtime and is freed the
	// moment this returns.
	print := make([]float32, Dimensions)
	copy(print, out.GetData())

	return normalise(print), nil
}
