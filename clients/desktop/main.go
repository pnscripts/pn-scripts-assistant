// Command pnexus-desktop is the native desktop application for Pnexus.
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
	"time"
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

	return fmt.Errorf("Pnexus did not respond within %s", startupTimeout)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

func main() {
	url := flag.String("url", envOr("PNEXUS_URL", defaultURL), "Pnexus URL to open")
	noStart := flag.Bool("no-start", false, "Don't try to start Pnexus if it isn't running")
	flag.Parse()

	if !brainIsUp(*url) {
		if *noStart {
			fmt.Fprintf(os.Stderr, "Pnexus is not running at %s\n", *url)
			os.Exit(1)
		}

		fmt.Println("Starting Pnexus...")

		if err := startBrain(); err != nil {
			fmt.Fprintf(os.Stderr, "Could not start Pnexus: %v\n", err)
			os.Exit(1)
		}

		if err := waitForBrain(*url); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	if err := openWindow(*url, "Pnexus", windowWidth, windowHeight); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
