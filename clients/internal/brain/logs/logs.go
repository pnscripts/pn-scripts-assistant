/*
 * Package logs is where this program writes down what it did.
 *
 * Until now everything went to standard error, which is right when somebody
 * started it in a terminal and useless the rest of the time: started from the
 * applications menu, standard error goes nowhere anybody can find. "It did not
 * start" was then a question with no way to answer it — the one moment the
 * record matters most is the one moment there was none.
 *
 * So: the same lines, to a file as well. Under XDG_STATE_HOME, which is where
 * this kind of thing belongs — not the data root, because the data root can be
 * a removable drive that is not plugged in, and a log that disappears with the
 * disk cannot explain why the disk is missing.
 *
 * Kept small by hand. A log nobody rotates is a disk somebody loses, and a
 * dependency for rotating one is a dependency this program does not need: at
 * five megabytes the file is renamed and a new one started, three kept.
 */
package logs

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"pn-scripts-assistant/internal/brain/paths"
)

const (
	// Biggest is when a log is rotated rather than appended to.
	Biggest = 5 << 20

	// Kept is how many older logs are left beside it.
	Kept = 3
)

// Folder is where the logs live: $XDG_STATE_HOME/pn-scripts-assistant/logs,
// or ~/.local/state/pn-scripts-assistant/logs when that is not set.
func Folder() string {
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, paths.Name, "logs")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), paths.Name, "logs")
	}

	return filepath.Join(home, ".local", "state", paths.Name, "logs")
}

// File is the log this run writes to.
func File() string { return filepath.Join(Folder(), paths.Name+".log") }

/*
 * Open starts a logger that writes to standard error and to the file, and
 * says where the file is.
 *
 * Both, not one or the other: somebody who started this in a terminal should
 * still see it there, and somebody who did not should still be able to read it
 * afterwards. A file that cannot be opened is not worth failing over — the
 * program runs, the terminal still has the lines, and the reason the file is
 * missing is itself logged.
 */
func Open(level slog.Level) (*slog.Logger, string) {
	to := io.Writer(os.Stderr)
	where := File()

	file, err := open(where)
	if err != nil {
		logger := slog.New(slog.NewTextHandler(to, &slog.HandlerOptions{Level: level}))
		logger.Warn("could not open the log file; this run is only in the terminal",
			"file", where, "error", err)

		return logger, ""
	}

	return slog.New(slog.NewTextHandler(io.MultiWriter(to, file), &slog.HandlerOptions{Level: level})), where
}

func open(where string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(where), 0o700); err != nil {
		return nil, err
	}

	if info, err := os.Stat(where); err == nil && info.Size() >= Biggest {
		rotate(where)
	}

	// 0600: what this program does is nobody else's business on a shared
	// machine, and a log of it is a record of the same thing.
	return os.OpenFile(where, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}

// rotate moves the log along by one and drops the oldest.
func rotate(where string) {
	os.Remove(fmt.Sprintf("%s.%d", where, Kept))

	for i := Kept - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", where, i), fmt.Sprintf("%s.%d", where, i+1))
	}

	os.Rename(where, where+".1")
}
