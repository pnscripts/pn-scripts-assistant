package browse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A page photographed with the browser that is on the machine.
 *
 * The program carries WebKit and uses it — in a build with cgo. A build
 * without cgo is a supported thing, and in it "can this show a picture of a
 * page" was false; the consequence was not a missing picture but a different
 * game engine, because the assistant weighs whether it can see what it built.
 *
 * Run against a real browser and a real page: a test that mocked one would be
 * testing the mock.
 */
func TestAPageIsPhotographedWithTheBrowserOnThisMachine(t *testing.T) {
	if browserHere() == "" {
		t.Skip("no Chrome or Chromium on this machine")
	}

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><head><title>A Little Game</title></head>
			<body><h1>It started</h1><canvas id="c" width="320" height="200"></canvas>
			<script>const c=document.getElementById('c').getContext('2d');
			c.fillStyle='#4dd0e1';c.fillRect(10,10,300,180);</script></body></html>`))
	}))
	defer page.Close()

	png := filepath.Join(t.TempDir(), "shot.png")

	shot, err := shotWithABrowser(context.Background(), page.URL, 2, png)
	if err != nil {
		t.Fatalf("it could not photograph a page: %v", err)
	}

	info, err := os.Stat(png)
	if err != nil {
		t.Fatalf("there is no picture at %s: %v", png, err)
	}

	if info.Size() < 1000 {
		t.Errorf("the picture is %d bytes, which is not a picture of anything", info.Size())
	}

	// It is a PNG, not an empty file with a PNG's name.
	head, err := os.ReadFile(png)
	if err != nil || len(head) < 8 || string(head[1:4]) != "PNG" {
		t.Errorf("the file is not a PNG")
	}

	if shot.Title != "A Little Game" {
		t.Errorf("the title came back as %q", shot.Title)
	}

	if shot.Canvas != 1 {
		t.Errorf("it counted %d canvases in a page with one", shot.Canvas)
	}

	if !strings.Contains(shot.Text, "It started") {
		t.Errorf("the words of the page came back as %q", shot.Text)
	}

	// And it says what it cannot do, rather than leaving somebody to find out.
	if !strings.Contains(shot.Trouble, "complained") {
		t.Errorf("it does not say what is missing: %q", shot.Trouble)
	}
}

/*
 * Showing a picture is possible if either way of doing it is here.
 *
 * This is the answer that decided a game engine: a web engine that cannot be
 * photographed loses to one that can, so a build without WebKit on a machine
 * with a browser was choosing Godot for the wrong reason.
 */
func TestShowingAPictureIsPossibleWithEither(t *testing.T) {
	if !webKitHere() && browserHere() == "" {
		t.Skip("neither WebKit nor a browser here")
	}

	if !Possible() {
		t.Error("it says it cannot show a picture while it can")
	}
}
