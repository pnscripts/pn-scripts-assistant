package speech

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

/*
 * Keeping the recording when a turn produced no words.
 *
 * "It did not hear me" has been diagnosed from numbers for two days — peak,
 * room floor, seconds of speech — and the numbers keep saying the audio was
 * fine. Nineteen seconds of speech at a peak of 1358 against a floor of 4 came
 * back empty, and the same levels reproduced from a file transcribe perfectly.
 * So the fault is in the recording itself, and no summary of it will show what
 * that is.
 *
 * The recording is the evidence. It is thrown away at the end of every turn,
 * which is correct for the ordinary case and exactly wrong for the case worth
 * investigating, so the failures are kept and everything else still goes.
 *
 * Deliberately small and self-limiting: a handful of files, oldest deleted, in
 * a folder anybody can open or empty. This is a microphone recording somebody's
 * home, so it stays on the machine, stays few, and stays where it can be seen.
 */

// HowManyFailuresToKeep is the size of the drawer.
const HowManyFailuresToKeep = 6

// FailedTurnsDir is where they go.
func FailedTurnsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".pn-brain", "unheard")
}

/*
 * KeepFailedTurn saves a recording that produced no words.
 *
 * Only when there was something to hear: a turn that was genuinely silence is
 * not a mystery and filling the drawer with those buries the ones that are.
 * Returns the path so it can be named in the interface, because a file nobody
 * knows about is the same as no file.
 */
func KeepFailedTurn(path string, spokeForMS, peak int) string {
	if spokeForMS < 1000 || peak < 400 {
		return ""
	}

	dir := FailedTurnsDir()
	if dir == "" {
		return ""
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}

	audio, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	// Named with what was measured, so the file itself says which case it is
	// without anybody having to match it against a log.
	name := fmt.Sprintf("%s-%dms-peak%d.wav",
		time.Now().Format("15-04-05"), spokeForMS, peak)

	kept := filepath.Join(dir, name)

	if err := os.WriteFile(kept, audio, 0o600); err != nil {
		return ""
	}

	tidyFailedTurns(dir)

	return kept
}

// tidyFailedTurns keeps the drawer small, oldest first out.
func tidyFailedTurns(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var wavs []os.DirEntry

	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".wav" {
			wavs = append(wavs, e)
		}
	}

	if len(wavs) <= HowManyFailuresToKeep {
		return
	}

	sort.Slice(wavs, func(i, j int) bool {
		a, _ := wavs[i].Info()
		b, _ := wavs[j].Info()

		if a == nil || b == nil {
			return wavs[i].Name() < wavs[j].Name()
		}

		return a.ModTime().Before(b.ModTime())
	})

	for _, old := range wavs[:len(wavs)-HowManyFailuresToKeep] {
		os.Remove(filepath.Join(dir, old.Name()))
	}
}
