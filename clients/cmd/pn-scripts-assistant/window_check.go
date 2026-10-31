package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"pn-scripts-assistant/internal/brain/window"
)

/*
 * window-check proves the native window works on this machine.
 *
 * Compiling for Windows proves nothing about a window: the COM calls are
 * reached by vtable index and only a real WebView2 answers them. So the
 * release opens a window on a page served here, on loopback only, waits for
 * WebView2 to say the page loaded, and fails the build if it did not. It
 * touches no brain, no settings and no network beyond 127.0.0.1.
 *
 * Exit status 0 means the page loaded; the line printed says which engine.
 */
func runWindowCheck(args []string) error {
	within := 90 * time.Second

	if len(args) > 0 {
		d, err := time.ParseDuration(args[0])
		if err != nil {
			return fmt.Errorf("window-check takes how long to wait, like 90s: %w", err)
		}

		within = d
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()

	page := http.NewServeMux()
	page.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><title>window check</title><p>PN Scripts Assistant: the window works.</p>")
	})

	srv := &http.Server{Handler: page, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)

	defer srv.Close()

	if err := window.Check("http://"+ln.Addr().String()+"/", within); err != nil {
		fmt.Fprintf(os.Stdout, "window check FAILED: %v\n", err)

		return err
	}

	fmt.Fprintf(os.Stdout, "window check OK: %s showed the page\n", window.Engine())

	return nil
}
