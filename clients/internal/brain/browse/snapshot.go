package browse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

/*
 * Shot is a page that was run rather than read: what it complained about while
 * it ran, whether it drew anything, and a picture of it.
 *
 * The same child process as reading a page, for the same reason — WebKit
 * wants the main thread, and whatever a page does wrong should happen to
 * something disposable. Used to check a web game that was just built: it
 * loaded, it ran for a few seconds, these errors, this is what it looked like.
 */
type Shot struct {
	Title   string   `json:"title"`
	Errors  []string `json:"errors"`
	Canvas  int      `json:"canvas"`
	Text    string   `json:"text"`
	Picture string   `json:"picture,omitempty"`

	// Trouble is what went wrong taking the picture, when the page itself
	// was read — a report without a picture is still a report.
	Trouble string `json:"trouble,omitempty"`
}

// Snapshot runs a page for some seconds and writes a PNG of it to png.
func Snapshot(ctx context.Context, url string, seconds int, png string) (Shot, error) {
	if !Possible() {
		return Shot{}, fmt.Errorf("this build cannot run pages: it was built without WebKit")
	}

	self, err := Program()
	if err != nil {
		return Shot{}, fmt.Errorf("could not find this program to run a page with: %w", err)
	}

	run, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second+75*time.Second)
	defer cancel()

	cmd := exec.CommandContext(run, self, "snapshot", url, strconv.Itoa(seconds), png)

	// Nothing of this brain goes with it; see Page.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.TempDir(),
		"DISPLAY=" + os.Getenv("DISPLAY"),
		"WAYLAND_DISPLAY=" + os.Getenv("WAYLAND_DISPLAY"),
		"XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"),
		"XAUTHORITY=" + os.Getenv("XAUTHORITY"),
	}

	var out, problems bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &problems

	runErr := cmd.Run()

	said, read := strings.CutPrefix(strings.TrimLeft(out.String(), "\n"), Opening+"\n")

	if !read {
		if run.Err() != nil {
			return Shot{}, fmt.Errorf("the page was still running after %s and was stopped", time.Duration(seconds)*time.Second+75*time.Second)
		}

		if runErr != nil {
			return Shot{}, fmt.Errorf("could not run the page: %s", why(problems.String(), runErr))
		}

		return Shot{}, fmt.Errorf("the page said nothing")
	}

	var shot Shot

	if err := json.Unmarshal([]byte(strings.TrimSpace(said)), &shot); err != nil {
		return Shot{}, fmt.Errorf("the page's report could not be read: %w", err)
	}

	if info, err := os.Stat(png); err == nil && info.Size() > 0 {
		shot.Picture = png
	} else {
		shot.Trouble = why(problems.String(), fmt.Errorf("no picture was written"))
	}

	return shot, nil
}
