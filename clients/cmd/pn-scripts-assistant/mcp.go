package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/release"
)

/*
 * runMCP is "pn-scripts-assistant mcp": this program as a Model Context
 * Protocol server for another program (Claude Code, Claude Desktop, Cursor)
 * that starts it and talks to it over its standard streams.
 *
 * It opens no brain and holds no key. Every tool is a question to the
 * assistant already running on this computer; with none running, the tools
 * say so. It lives exactly as long as the program that started it keeps
 * its input open.
 *
 * Standard output is the conversation: nothing but messages may be written
 * there, so everything meant for a person goes to standard error.
 */
func runMCP(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return serveMCP(ctx, args, os.Stdin, os.Stdout, os.Stderr)
}

func serveMCP(ctx context.Context, args []string, in io.Reader, out, said io.Writer) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(said)
	fs.Usage = func() {
		fmt.Fprint(said, "pn-scripts-assistant mcp lets another program (Claude Code, Claude Desktop, Cursor) ask\n"+
			"PN Scripts Assistant running on this computer. That program starts it; it is not for typing into.\n\n")
		fs.PrintDefaults()
	}

	api := fs.String("api", mcpserve.Address(os.Getenv), "address of PN Scripts Assistant on this computer")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("pn-scripts-assistant mcp takes no arguments besides --api (got %q)", fs.Arg(0))
	}

	address, err := mcpserve.Loopback(*api)
	if err != nil {
		return err
	}

	server := mcpserve.New(address, release.Is())
	server.Log = said

	return server.Serve(ctx, in, out)
}
