//go:build !linux

package main

import (
	"fmt"
	"runtime"
)

// Windows (WebView2) and macOS (WKWebView) each need their own native
// implementation. They are not written yet, and this fails loudly rather than
// quietly falling back to a browser — an app that silently opens Chrome is the
// thing this client exists to avoid.
func openWindow(url, title string, width, height int) error {
	return fmt.Errorf(
		"the native window is not implemented for %s yet; open %s in a browser meanwhile",
		runtime.GOOS, url,
	)
}
