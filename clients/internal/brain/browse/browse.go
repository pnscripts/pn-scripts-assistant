/*
 * Package browse opens a page the way a browser opens it.
 *
 * The plain fetch reads what the server sent, which for a great many sites in
 * 2026 is an empty shell and a script tag. This runs the scripts and reads
 * what a person would actually see.
 *
 * It does that in a child process, and that is the whole design rather than an
 * implementation detail. WebKit and GTK both want the main thread, neither
 * forgives being driven from anywhere else, and this program already has scars
 * from getting that wrong — an X error arriving asynchronously once aborted
 * the entire process at teardown, and whether it died was a race. Run as a
 * child, the worst a hostile or merely broken page can do is kill something
 * disposable, and the assistant finds out by getting an error back.
 */
package browse

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Patience is how long a page gets before whatever it has rendered is taken as
// the answer.
const Patience = 30 * time.Second

/*
 * Marker is how the child says what actually went wrong.
 *
 * An offscreen WebKit prints GTK warnings on every run — harmless, and the
 * last thing on stderr. Taking the last line as the reason meant a page that
 * failed to load reported "drawable is not a native X11 window", which is
 * true, unrelated and no help at all.
 */
const Marker = "page-error: "

/*
 * Opening is the first line of a page the child read.
 *
 * Without it, anything the child happened to print was taken for a page. That
 * is not hypothetical: run from a test binary, os.Executable is the test
 * binary — so "render https://example.com" ran the test suite, and the word
 * PASS came back as the contents of the page. In a real build the executable
 * is this program, but a rule that holds only when nothing unusual happens is
 * not a rule, and the failure it produces is the worst kind: confident,
 * plausible, and wrong.
 */
const Opening = "--- page ---"

/*
 * Program is which executable renders a page. Overridable so the path can be
 * tested; in every real build it is this program, with its hidden subcommand.
 */
var Program = os.Executable

// MostText is as much of a page as comes back. The same cap the plain fetch
// uses, for the same reason: a page is not a document, and forty thousand
// characters is already more than any answer needs.
const MostText = 40000

/*
 * Page loads a URL in a child process and returns what it says.
 *
 * The caller has already checked the address — see the tool, which uses the
 * same guard the plain fetch does. Nothing here should be reached with an
 * address that was not checked, and nothing here checks it again, because two
 * places deciding what is safe is how they come to disagree.
 */
func Page(ctx context.Context, url string) (string, error) {
	if !Possible() {
		return "", fmt.Errorf(
			"this build cannot open pages: it was built without WebKit, so only the " +
				"plain fetch is available")
	}

	self, err := Program()
	if err != nil {
		return "", fmt.Errorf("could not find this program to open a page with: %w", err)
	}

	// A little longer than the child's own patience, so the child gets to
	// report what it managed rather than being killed a moment before it
	// would have answered.
	run, cancel := context.WithTimeout(ctx, Patience+20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(run, self, "render", url,
		strconv.Itoa(int(Patience.Seconds())))

	/*
	 * Nothing of this brain goes with it.
	 *
	 * A fresh environment rather than the parent's: the child has no reason to
	 * know where the brain's data is, which providers have keys, or anything
	 * else that happens to be exported — and it is the one process here that
	 * runs somebody else's code.
	 */
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

	err = cmd.Run()

	text, read := strings.CutPrefix(strings.TrimLeft(out.String(), "\n"), Opening+"\n")

	if read {
		text = strings.TrimSpace(text)
	}

	/*
	 * Read means the child got as far as saying so, even if it then died on
	 * its way out.
	 *
	 * That happens — an offscreen WebKit can fall over during teardown — and
	 * throwing away a page because its renderer stumbled while closing would
	 * be losing the answer over the tidying up.
	 */
	if read && text != "" {
		return trim(text), nil
	}

	if !read && out.Len() > 0 && err == nil {
		return "", fmt.Errorf(
			"this program could not start its own browser: what answered was not it")
	}

	if err != nil {
		if run.Err() != nil {
			return "", fmt.Errorf("that page took longer than %s to show anything", Patience)
		}

		return "", fmt.Errorf("could not open that page: %s", why(problems.String(), err))
	}

	return "", fmt.Errorf("that page came back empty")
}

func why(problems string, err error) string {
	for _, line := range strings.Split(problems, "\n") {
		if reason, found := strings.CutPrefix(strings.TrimSpace(line), Marker); found {
			return reason
		}
	}

	// Nothing marked, so the child died rather than reporting. Its own chatter
	// is no use here, and the exit status at least says it did not finish.
	return err.Error()
}

func trim(text string) string {
	// Blank lines collapsed: a rendered page is mostly empty space where the
	// layout was, and sending that to a model is paying for whitespace.
	var kept []string

	blank := 0

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t")

		if strings.TrimSpace(line) == "" {
			blank++

			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}

		kept = append(kept, line)
	}

	out := strings.Join(kept, "\n")

	if len(out) <= MostText {
		return out
	}

	return out[:MostText] + "\n\n[…the rest of the page was longer than is worth reading]"
}
