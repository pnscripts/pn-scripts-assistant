package browse

import (
	"os"
	"strings"
	"testing"
)

// A headless browser is started without sound and without speech synthesis.
func TestHeadlessBrowsersAreMuted(t *testing.T) {
	for _, want := range []string{"--mute-audio", "--disable-speech-api", "--disable-speech-synthesis-api"} {
		found := false

		for _, f := range Muted {
			found = found || f == want
		}

		if !found {
			t.Errorf("Muted lacks %s", want)
		}
	}

	for _, file := range []string{"withabrowser.go", "../tools/browser.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(raw), "--headless") && !strings.Contains(string(raw), "Muted...") {
			t.Errorf("%s starts a headless browser without the Muted flags", file)
		}
	}
}
