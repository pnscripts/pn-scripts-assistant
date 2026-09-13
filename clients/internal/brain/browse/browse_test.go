package browse

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// asProgram points the renderer at a small script standing in for the child.
func asProgram(t *testing.T, script string) {
	t.Helper()

	// Page refuses outright on a build with no WebKit, which is correct and
	// leaves nothing here to test.
	if !Possible() {
		t.Skip("this build has no WebKit, so there is no renderer to stand in for")
	}

	path := filepath.Join(t.TempDir(), "stand-in")

	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	was := Program

	Program = func() (string, error) { return path, nil }

	t.Cleanup(func() { Program = was })
}

/*
 * Something that is not the renderer is not a page.
 *
 * This is not hypothetical. Run from a test binary, os.Executable is the test
 * binary — so asking it to render a page ran the test suite instead, and the
 * word PASS came back as the contents of example.com. The failure is the worst
 * kind: confident, plausible, and wrong, with nothing anywhere saying so.
 */
func TestSomethingThatIsNotTheRendererIsNotAPage(t *testing.T) {
	asProgram(t, `echo PASS`)

	_, err := Page(context.Background(), "https://example.com/")
	if err == nil {
		t.Fatal("whatever answered was taken for a web page")
	}

	if !strings.Contains(err.Error(), "could not start its own browser") {
		t.Errorf("it said %v", err)
	}
}

// A page that was read comes back, even when the renderer then falls over on
// its way out — which an offscreen WebKit does.
func TestAPageReadSurvivesAMessyExit(t *testing.T) {
	asProgram(t, "echo '"+Opening+"'\necho 'the page said this'\nexit 3")

	text, err := Page(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("a page that was read was thrown away: %v", err)
	}

	if !strings.Contains(text, "the page said this") {
		t.Errorf("it came back as %q", text)
	}
}

/*
 * What actually went wrong is reported, not GTK's chatter.
 *
 * An offscreen WebKit prints warnings about drawables on every single run, and
 * they are the last thing on stderr. Taking the last line meant a page that
 * failed to load reported "drawable is not a native X11 window" — true,
 * unrelated, and no help to anybody.
 */
func TestTheRealReasonIsReportedNotTheChatter(t *testing.T) {
	asProgram(t, "echo '"+Marker+"the page would not load' >&2\n"+
		"echo 'Gdk-WARNING: drawable is not a native X11 window' >&2\nexit 1")

	_, err := Page(context.Background(), "https://example.com/")
	if err == nil {
		t.Fatal("a page that failed was reported as read")
	}

	if !strings.Contains(err.Error(), "would not load") {
		t.Errorf("it said %v", err)
	}

	if strings.Contains(err.Error(), "drawable") {
		t.Errorf("it reported GTK's chatter as the reason: %v", err)
	}
}

// A page of mostly empty layout is not sent on as mostly empty layout.
func TestBlankSpaceIsNotWorthSending(t *testing.T) {
	text := trim("Title\n\n\n\n\n\nSome words\n\n\n\nMore words")

	if strings.Contains(text, "\n\n\n") {
		t.Errorf("the empty space came through: %q", text)
	}

	if !strings.Contains(text, "Some words") || !strings.Contains(text, "More words") {
		t.Errorf("the words did not: %q", text)
	}
}
