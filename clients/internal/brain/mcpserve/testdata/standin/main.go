// A stand-in for "pn-scripts-assistant mcp" that a test can build without
// the window's libraries: the same server on the same streams, pointed at
// the address the test gives it.
package main

import (
	"context"
	"fmt"
	"os"

	"pn-scripts-assistant/internal/brain/mcpserve"
)

func main() {
	address, err := mcpserve.Loopback(mcpserve.Address(os.Getenv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	server := mcpserve.New(address, "stand-in")
	server.Log = os.Stderr

	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
