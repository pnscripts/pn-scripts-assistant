package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/browse"
)

// The whole path, through the tool, against a real page. Skipped unless asked
// for, because a test that needs the internet is a test that fails on a train.
func TestTheBrowserOpensARealPage(t *testing.T) {
	if os.Getenv("PN_SCRIPTS_ASSISTANT_ONLINE_TESTS") == "" {
		t.Skip("set PN_SCRIPTS_ASSISTANT_ONLINE_TESTS=1 to run the test that reaches the web")
	}

	// The built program, because os.Executable in a test binary is the test
	// binary — which is how the word PASS once came back as a web page.
	built, err := filepath.Abs("../../../dist/pn-scripts-assistant")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(built); err != nil {
		t.Skip("build dist/pn-scripts-assistant first: this test drives the real renderer")
	}

	browse.Program = func() (string, error) { return built, nil }

	raw, _ := json.Marshal(pageArgs{URL: "https://example.com/"})

	var tool ReadAPage

	out, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("the page would not open: %v", err)
	}

	if !strings.Contains(strings.ToLower(out), "example domain") {
		t.Errorf("the page did not come back: %q", out)
	}
}
