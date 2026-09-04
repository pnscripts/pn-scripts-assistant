package speech

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

/*
 * A note of what was turned down, kept on the disk while it is down.
 *
 * Everything else here is careful to put the level back — on the timer, when
 * the setting is switched off, when the program stops. None of that survives
 * the program being killed, and the consequence of missing it is not a
 * momentary glitch: WirePlumber remembers a stream's volume against the
 * application, so a browser left at a fifth stays at a fifth tomorrow, and
 * next week, with nothing anywhere to suggest what did it.
 *
 * That is the kind of fault that makes somebody uninstall a program without
 * ever finding out it was the cause. So the note is written before the level
 * is changed and removed after it is put back, and anything found at startup
 * is restored.
 */

// DuckedNote is where the note lives: beside the machine's other local state,
// not in the brain's data, because it describes this machine's speakers rather
// than anything the brain knows.
func duckedNotePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".pn-brain", "turned-down.json")
}

func rememberDucked(levels map[string]float64) {
	path := duckedNotePath()

	if path == "" || len(levels) == 0 {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	raw, err := json.Marshal(levels)
	if err != nil {
		return
	}

	os.WriteFile(path, raw, 0o600)
}

func forgetDucked() {
	if path := duckedNotePath(); path != "" {
		os.Remove(path)
	}
}

/*
 * PutBackAnythingLeftDown restores levels from a previous run that ended badly.
 *
 * Called at startup. Silent when there is nothing to do, which is almost
 * always: the note only exists if the program stopped between turning
 * something down and putting it back.
 */
func PutBackAnythingLeftDown() {
	path := duckedNotePath()

	if path == "" {
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var levels map[string]float64

	if err := json.Unmarshal(raw, &levels); err != nil {
		os.Remove(path)

		return
	}

	if len(levels) > 0 && haveVolumeControl() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		putBack(ctx, levels)
	}

	os.Remove(path)
}
