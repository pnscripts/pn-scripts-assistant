package display

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

/*
 * When mutter cannot open its display, the reason it printed is read while
 * it may still be writing.
 *
 * Found by `make race` on 10.11.2026: with a temporary folder whose path is
 * too long for a socket, mutter fails with "Failed to create_socket", and
 * Start read its output while the copy from mutter's pipe was writing into
 * it. The race detector failed the Godot photograph test for it.
 */
func TestMutterFailingToOpenADisplayIsReportedWithoutARace(t *testing.T) {
	if kind, err := Available(); err != nil || kind != Mutter {
		t.Skip("mutter is not installed here")
	}

	// A socket path longer than the 108 bytes a Unix socket allows.
	long := filepath.Join(t.TempDir(), strings.Repeat("x", 120))
	if err := os.MkdirAll(long, 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TMPDIR", long)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	v, err := Start(ctx, 320, 240)
	if err == nil {
		v.Stop()
		t.Skip("mutter opened a display even with a long socket path")
	}

	if !strings.Contains(err.Error(), "mutter") && !strings.Contains(err.Error(), "bus") {
		t.Errorf("the reason does not say what failed: %v", err)
	}

	if entries, _ := os.ReadDir(long); len(entries) != 0 {
		t.Errorf("the display's folder was left behind: %d entries", len(entries))
	}
}

// What a program says is written by one goroutine and read by another.
func TestWhatItSaidIsSafeToReadWhileItIsWritten(t *testing.T) {
	var said output

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 1000; i++ {
			said.Write([]byte("line\n"))
		}
	}()

	for i := 0; i < 1000; i++ {
		_ = said.String()
	}

	wg.Wait()

	if got := strings.Count(said.String(), "line\n"); got != 1000 {
		t.Errorf("%d lines kept, want 1000", got)
	}
}
