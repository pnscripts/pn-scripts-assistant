// A stand-in for "pn-scripts-assistant mcp" that a test can build without
// the window's libraries: the same server on the same streams, pointed at
// the address the test gives it.
package main

import (
	"context"
	"fmt"
	"os"

	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/vouch"
)

func main() {
	address, err := mcpserve.Loopback(mcpserve.Address(os.Getenv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// The real program reads the key from the account's settings folder
	// (vouch.Here). A test names a file instead, so that it needs no
	// folder of the account it runs under, on any system.
	key := vouch.Here
	if file := os.Getenv("STANDIN_KEY_FILE"); file != "" {
		key = func() ([]byte, error) { return vouch.Read(file) }
	}

	server := mcpserve.New(address, "stand-in", key)
	server.Log = os.Stderr

	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
