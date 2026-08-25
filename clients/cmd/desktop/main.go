// Command pn-brain-desktop is the native desktop application for PN Brain.
//
// It renders in the operating system's own web engine — WebKitGTK on Linux —
// so this is a real application window, not a browser wearing a disguise. No
// Chrome, no bundled Chromium, no tabs, no address bar.
//
// The trade this makes, stated plainly: the window needs cgo, so the binary can
// no longer be cross-compiled for every platform from one Linux machine. Each
// target has to be built on its own OS, which is what the release workflow now
// does with native runners. The alternative — bundling Chromium via Electron —
// costs around 150MB per platform and still isn't native.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"pn-brain/internal/preflight"
	"pn-brain/internal/setup"
)

const (
	defaultURL     = "http://localhost:8090"
	healthPath     = "/api/brain"
	startupTimeout = 90 * time.Second
	windowWidth    = 1200
	windowHeight   = 820
)

func brainIsUp(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(url + healthPath)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// startBrain runs the launcher that brings the containers up. It lives beside
// this binary in the repo, so the path is derived from the executable rather
// than hardcoded to one machine.
func startBrain() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	script := filepath.Join(filepath.Dir(exe), "..", "..", "scripts", "start-brain.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("launcher script not found at %s", script)
	}

	cmd := exec.Command(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func waitForBrain(url string) error {
	deadline := time.Now().Add(startupTimeout)

	for time.Now().Before(deadline) {
		if brainIsUp(url) {
			return nil
		}

		time.Sleep(time.Second)
	}

	return fmt.Errorf("PN Brain did not respond within %s", startupTimeout)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// needsSetup reports whether the machine is missing something PN Brain needs, or
// has no way to think at all.
//
// The second half matters as much as the first: every dependency can be present
// and the app still be useless, because there is neither a local model nor an
// API key. Someone in that position needs to be asked a question, not shown a
// list of green ticks followed by a broken chat.
func needsSetup(envPath string) bool {
	results := preflight.Check()

	if preflight.BlockingCount(results) > 0 {
		return true
	}

	for _, r := range results {
		if r.Requirement.Name == "Chat model" && r.Satisfied() {
			return false
		}
	}

	return !hasAPIKeyIn(envPath)
}

func hasAPIKeyIn(envPath string) bool {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ANTHROPIC_API_KEY=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "ANTHROPIC_API_KEY=")) != ""
		}
	}

	return false
}

// runSetup shows first-run setup inside the application window.
//
// The window opens on a server this binary runs itself, because the brain
// cannot serve its own setup page on a machine that is missing the things the
// brain needs in order to start. Once the user is done, the same window
// navigates to the brain — so setup happens entirely inside the program, with
// no terminal involved.
func runSetup(envPath, brainURL string) error {
	server, err := setup.New(envPath)
	if err != nil {
		return fmt.Errorf("could not start setup: %w", err)
	}

	navigate := make(chan string, 1)

	go server.Serve(func() {
		if err := startBrain(); err != nil {
			return
		}

		if err := waitForBrain(brainURL); err != nil {
			return
		}

		navigate <- brainURL
	})

	return openWindowWithNavigation(server.URL(), "PN Brain — Setup", windowWidth, windowHeight, navigate)
}

// defaultEnvPath finds the .env beside the repo, derived from the binary's
// location rather than hardcoded.
func defaultEnvPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ".env"
	}

	return filepath.Join(filepath.Dir(exe), "..", ".env")
}

func main() {
	url := flag.String("url", envOr("PN_BRAIN_URL", defaultURL), "PN Brain URL to open")
	noStart := flag.Bool("no-start", false, "Don't try to start PN Brain if it isn't running")
	skipSetup := flag.Bool("skip-setup", false, "Skip the first-run check")
	flag.Parse()

	envPath := defaultEnvPath()

	if !*skipSetup && !brainIsUp(*url) && needsSetup(envPath) {
		if err := runSetup(envPath, *url); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		return
	}

	if !brainIsUp(*url) {
		if *noStart {
			fmt.Fprintf(os.Stderr, "PN Brain is not running at %s\n", *url)
			os.Exit(1)
		}

		fmt.Println("Starting PN Brain...")

		if err := startBrain(); err != nil {
			fmt.Fprintf(os.Stderr, "Could not start PN Brain: %v\n", err)
			os.Exit(1)
		}

		if err := waitForBrain(*url); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	if err := openWindow(*url, "PN Brain", windowWidth, windowHeight); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
