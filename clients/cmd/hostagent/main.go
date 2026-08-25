// Command pn-brain-hostagent runs the host-side agent.
//
// The brain lives in a container and can therefore only see what is mounted
// into it. This runs outside that container, on the machine itself, and gives
// the brain reach — reading anywhere, and running commands — through a door the
// permission gate still controls.
//
// It refuses to start without a token. There is no default and no generated
// fallback: a daemon that can read any file and run any command must never be
// reachable by anything that merely knows the port.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pn-brain/internal/hostagent"
)

const defaultAddr = "127.0.0.1:8791"

func main() {
	addr := flag.String("listen", envOr("PN_BRAIN_AGENT_ADDR", defaultAddr), "address to listen on")
	flag.Parse()

	token := os.Getenv("PN_BRAIN_AGENT_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr,
			"PN_BRAIN_AGENT_TOKEN is not set.\n\n"+
				"This agent can read any file on this machine and run commands on it, so it\n"+
				"will not start without a shared secret. Generate one and put it in the\n"+
				"brain's .env as PN_BRAIN_AGENT_TOKEN:\n\n"+
				"  openssl rand -hex 32")
		os.Exit(1)
	}

	// Bound explicitly to loopback, and refuses anything else. Passing a public
	// address is not a configuration choice worth honouring; it would put a
	// shell on the network.
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		fmt.Fprintf(os.Stderr, "refusing to listen on %q: this agent binds loopback only\n", *addr)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              *addr,
		Handler:           hostagent.New(token).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("PN Brain host agent listening on %s\n", *addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "agent stopped: %v\n", err)
			os.Exit(1)
		}
	}()

	<-stop

	// Give an in-flight command a moment to finish rather than cutting it off
	// half-done, which is worse than either finishing or never starting.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = server.Shutdown(ctx)
	fmt.Println("\nhost agent stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
