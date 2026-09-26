package browse

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/*
 * Photographing a page with whatever browser is on the machine.
 *
 * The program carries WebKit and uses it for this — but only in a build with
 * cgo, and a build without cgo is a supported thing: the README offers it,
 * and it is what somebody gets who builds this without the GTK headers. In
 * that build "can this program show a picture of a web page" was false, and
 * the consequence was not a missing picture. It was that asking for a game
 * quietly got a different engine: the assistant weighs whether it can *see*
 * what it built, so a web engine it cannot photograph loses to Godot, and on
 * a machine with no Godot the whole request stops.
 *
 * A browser can do it, and every machine that runs this has one. Chrome and
 * Chromium will take a picture from the command line with no protocol and no
 * library — which is fewer moving parts than the WebKit path, and available
 * in every build.
 *
 * It is the second choice, not the first: WebKit reports what the page
 * complained about while it ran, and this does not. A picture and an honest
 * note about what is missing beats no picture and a different engine.
 */

// HowLongAPageMayTake bounds the browser. A page that has not drawn anything
// in this long has not drawn anything.
const HowLongAPageMayTake = 90 * time.Second

/*
 * browsers are the names a browser is installed under, in the order they are
 * tried.
 *
 * Chrome first because its headless mode is what this uses; Firefox has one
 * too, and takes a different flag, so it is handled separately below.
 */
var browsers = []string{
	"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
}

// browserHere is the browser to photograph pages with, or empty.
func browserHere() string {
	for _, name := range browsers {
		if at, err := exec.LookPath(name); err == nil {
			return at
		}
	}

	return ""
}

/*
 * shotWithABrowser runs a page in a headless browser and writes a PNG of it.
 *
 * Two runs of the same browser rather than one: --screenshot takes the
 * picture, --dump-dom says what the page turned into. Both are a single
 * command with no protocol to speak, which is the point of using them — the
 * alternative is a debugging protocol over a websocket, and this is a
 * fallback rather than the main path.
 */
func shotWithABrowser(ctx context.Context, url string, seconds int, png string) (Shot, error) {
	browser := browserHere()
	if browser == "" {
		return Shot{}, fmt.Errorf("this build has no WebKit in it and there is no browser " +
			"on this machine to use instead — install Chromium, or build with cgo")
	}

	if seconds <= 0 {
		seconds = 3
	}

	if err := os.MkdirAll(filepath.Dir(png), 0o700); err != nil {
		return Shot{}, fmt.Errorf("making somewhere to put the picture: %w", err)
	}

	ctx, stop := context.WithTimeout(ctx, HowLongAPageMayTake)
	defer stop()

	// Let the page run for as long as it was given, then photograph it.
	// Virtual time, so a page that animates does not decide how long this
	// takes.
	budget := strconv.Itoa(seconds * 1000)

	profile, err := os.MkdirTemp("", "pn-page-")
	if err != nil {
		return Shot{}, err
	}

	defer os.RemoveAll(profile)

	common := []string{
		"--headless=new", "--no-first-run", "--no-default-browser-check",
		"--disable-gpu", "--hide-scrollbars", "--user-data-dir=" + profile,
		"--virtual-time-budget=" + budget, "--window-size=1280,900",
	}

	shot := Shot{}

	picture := exec.CommandContext(ctx, browser, append(append([]string{}, common...),
		"--screenshot="+png, url)...)

	if out, err := picture.CombinedOutput(); err != nil {
		shot.Trouble = strings.TrimSpace(lastLine(string(out)))

		if shot.Trouble == "" {
			shot.Trouble = err.Error()
		}
	} else {
		shot.Picture = png
	}

	/*
	 * What the page became, which is how "it loaded" is told from "it drew
	 * something": a game that never starts leaves an empty body and no
	 * canvas, and a picture of a white rectangle looks the same either way.
	 */
	dom := exec.CommandContext(ctx, browser, append(append([]string{}, common...),
		"--dump-dom", url)...)

	if out, err := dom.Output(); err == nil {
		page := string(out)

		shot.Canvas = strings.Count(page, "<canvas")
		shot.Title = between(page, "<title>", "</title>")
		shot.Text = readable(page)
	}

	/*
	 * And the one thing this cannot do, said rather than left for somebody to
	 * discover: a browser driven this way does not hand back what the page
	 * complained about while it ran.
	 */
	if shot.Trouble == "" {
		shot.Trouble = "photographed with " + filepath.Base(browser) +
			"; it does not report what the page complained about while it ran"
	}

	if shot.Picture == "" {
		return shot, fmt.Errorf("the browser did not produce a picture: %s", shot.Trouble)
	}

	return shot, nil
}

// between is what lies between two markers, or empty.
func between(text, from, to string) string {
	start := strings.Index(text, from)
	if start < 0 {
		return ""
	}

	rest := text[start+len(from):]

	end := strings.Index(rest, to)
	if end < 0 {
		return ""
	}

	return strings.TrimSpace(rest[:end])
}

/*
 * readable is the words out of a page's markup, roughly.
 *
 * Rough on purpose: this is for a model to read as "the page said this", not
 * for anything to parse. A tag-stripper of twenty lines is the right size for
 * that, and a parser is a dependency bought for a fallback.
 */
func readable(page string) string {
	var out strings.Builder

	inTag, inScript := false, false

	for i := 0; i < len(page); i++ {
		switch {
		case strings.HasPrefix(page[i:], "<script"), strings.HasPrefix(page[i:], "<style"):
			inScript = true
			inTag = true
		case inScript && strings.HasPrefix(page[i:], "</script>"), inScript && strings.HasPrefix(page[i:], "</style>"):
			inScript = false
		case page[i] == '<':
			inTag = true
		case page[i] == '>':
			inTag = false
		case !inTag && !inScript:
			out.WriteByte(page[i])
		}
	}

	words := strings.Fields(out.String())

	if len(words) > 400 {
		words = words[:400]
	}

	return strings.Join(words, " ")
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	return lines[len(lines)-1]
}

// jsonShot is here so the shape stays beside the thing that fills it.
var _ = json.Marshal
