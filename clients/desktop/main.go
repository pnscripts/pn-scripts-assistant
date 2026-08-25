// Command pnexus-desktop is the cross-platform desktop shell for Pnexus.
//
// Why this rather than Electron or Tauri:
//
//   - NativePHP/Electron requires illuminate/contracts ^10|^11|^12 and would
//     force a Laravel 13 downgrade, plus ~150MB of bundled Chromium.
//   - Tauri needs a Rust toolchain that isn't installed, and a native webview
//     needs libwebkit2gtk + GTK dev headers installed as root.
//
// Every desktop machine already has a Chromium-family browser, and every one of
// them supports `--app=`, which opens a chromeless window with no tabs, no URL
// bar, and its own taskbar entry. Driving that from Go gives a real app window
// with no new dependencies and a binary measured in megabytes.
//
// The trade is honest: this needs a Chromium-family browser present. On Windows
// that is guaranteed (Edge); on Linux and macOS it is near-universal but not
// certain, so a missing browser is reported rather than silently falling back to
// a normal tab.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	defaultURL     = "http://localhost:8090"
	healthPath     = "/api/brain"
	startupTimeout = 90 * time.Second
)

// candidates lists Chromium-family browsers per platform, best first. Edge is
// included for Windows because it ships with the OS.
func candidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	default:
		return []string{
			"google-chrome", "google-chrome-stable", "chromium",
			"chromium-browser", "brave-browser", "microsoft-edge",
		}
	}
}

func findBrowser() (string, error) {
	for _, c := range candidates() {
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
			continue
		}
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no Chromium-family browser found (tried: %v)", candidates())
}

func brainIsUp(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + healthPath)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// startBrain runs the launcher script that brings the containers up. It lives
// beside this binary in the repo, so the path is derived from the executable
// rather than hardcoded.
func startBrain() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// clients/desktop/<binary> -> ../../scripts/start-brain.sh
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
	return fmt.Errorf("brain did not respond within %s", startupTimeout)
}

func main() {
	url := flag.String("url", envOr("PNEXUS_URL", defaultURL), "Pnexus URL to open")
	noStart := flag.Bool("no-start", false, "Don't try to start the brain if it isn't running")
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

	browser, err := findBrowser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// A dedicated profile directory keeps the app window out of the user's
	// normal browsing session: its own cookies and history, its own taskbar
	// entry, and no chance of the app appearing as a tab in an existing window.
	profile, err := os.UserConfigDir()
	if err != nil {
		profile = os.TempDir()
	}
	profile = filepath.Join(profile, "pnexus", "app-profile")

	args := []string{
		"--app=" + *url,
		"--user-data-dir=" + profile,
		"--window-size=1200,820",
		"--no-first-run",
		"--no-default-browser-check",
	}

	fmt.Printf("Opening Pnexus (%s)\n", filepath.Base(browser))

	cmd := exec.Command(browser, args...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open window: %v\n", err)
		os.Exit(1)
	}

	// Stay attached so closing the window ends this process too, which keeps the
	// desktop entry's lifecycle honest.
	_ = cmd.Wait()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
