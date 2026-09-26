/*
 * Package display is a screen nobody can see, for taking a picture of
 * something that only draws when it has one.
 *
 * A game engine run headless draws nothing, and a game run on somebody's real
 * screen puts a window in front of whatever they were doing. What a check
 * needs is in between: a display that exists, is private to one run, and is
 * gone afterwards.
 *
 * Two ways to have one, tried in order. GNOME's own compositor, mutter, runs
 * headless with a virtual monitor and needs nothing installed on a GNOME
 * desktop — it is given a bus of its own that starts no services, because the
 * desktop portal it would otherwise wake waits half a minute for a keyring
 * that is not coming. Failing that, xvfb-run, when somebody has installed it.
 * Failing both, the answer is that there is none, and why.
 */
package display

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/sandbox"
	"strings"
	"sync/atomic"
	"time"
)

// Kind is how a display is made.
type Kind string

const (
	Mutter Kind = "mutter"
	Xvfb   Kind = "xvfb"
)

// Virtual is one private display, running until Stop.
type Virtual struct {
	Kind Kind

	// Env is what a program is given to draw on it.
	Env []string

	// Wrap is put in front of a command, for kinds that start the program
	// themselves rather than lend it a display.
	Wrap []string

	runtime string
	stops   []*exec.Cmd
}

// Available says which kind of display this machine can make, or why none.
func Available() (Kind, error) {
	if _, err := exec.LookPath("mutter"); err == nil {
		if _, err := exec.LookPath("dbus-daemon"); err == nil {
			return Mutter, nil
		}
	}

	if _, err := exec.LookPath("xvfb-run"); err == nil {
		return Xvfb, nil
	}

	return "", errors.New("there is no way to make a private display here: " +
		"mutter (GNOME's compositor) is not installed, and neither is xvfb-run (the xvfb recipe)")
}

var made atomic.Int64

// busConfig is a session bus that activates nothing: every name somebody
// asks for that is not already on it fails at once instead of waiting.
const busConfig = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-BUS Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:tmpdir=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

// Start makes a display of the given size.
func Start(ctx context.Context, width, height int) (*Virtual, error) {
	kind, err := Available()
	if err != nil {
		return nil, err
	}

	if kind == Xvfb {
		return &Virtual{Kind: Xvfb, Wrap: []string{"xvfb-run", "-a", "-s",
			fmt.Sprintf("-screen 0 %dx%dx24", width, height)}}, nil
	}

	runtime, err := os.MkdirTemp("", "pn-display-")
	if err != nil {
		return nil, err
	}

	v := &Virtual{Kind: Mutter, runtime: runtime}

	fail := func(err error) (*Virtual, error) {
		v.Stop()

		return nil, err
	}

	os.Chmod(runtime, 0o700)

	config := filepath.Join(runtime, "bus.conf")
	if err := os.WriteFile(config, []byte(fmt.Sprintf(busConfig, runtime)), 0o600); err != nil {
		return fail(err)
	}

	bus := exec.Command("dbus-daemon", "--config-file="+config, "--nofork", "--print-address=1")
	bus.Env = base(runtime)
	// Its own process group, however this system makes one. See sandbox.
	sandbox.OwnGroup(bus)

	out, err := bus.StdoutPipe()
	if err != nil {
		return fail(err)
	}

	if err := bus.Start(); err != nil {
		return fail(fmt.Errorf("could not start a private bus: %w", err))
	}

	v.stops = append(v.stops, bus)

	address := make(chan string, 1)

	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
		address <- strings.TrimSpace(line)
	}()

	var addr string

	select {
	case addr = <-address:
	case <-time.After(5 * time.Second):
		return fail(errors.New("the private bus did not start"))
	}

	name := fmt.Sprintf("pn-display-%d-%d", os.Getpid(), made.Add(1))

	env := append(base(runtime), "DBUS_SESSION_BUS_ADDRESS="+addr)

	compositor := exec.Command("mutter", "--headless", "--wayland", "--no-x11",
		"--virtual-monitor", fmt.Sprintf("%dx%d", width, height), "--wayland-display", name)
	compositor.Env = env
	sandbox.OwnGroup(compositor)

	var said strings.Builder

	compositor.Stdout, compositor.Stderr = &said, &said

	if err := compositor.Start(); err != nil {
		return fail(fmt.Errorf("could not start mutter: %w", err))
	}

	v.stops = append([]*exec.Cmd{compositor}, v.stops...)

	socket := filepath.Join(runtime, name)

	for deadline := time.Now().Add(10 * time.Second); ; {
		if _, err := os.Stat(socket); err == nil {
			break
		}

		if time.Now().After(deadline) || ctx.Err() != nil {
			return fail(fmt.Errorf("mutter did not open a display: %s", lastLine(said.String())))
		}

		time.Sleep(50 * time.Millisecond)
	}

	v.Env = []string{"XDG_RUNTIME_DIR=" + runtime, "WAYLAND_DISPLAY=" + name,
		"DBUS_SESSION_BUS_ADDRESS=" + addr, "XDG_SESSION_TYPE=wayland"}

	return v, nil
}

// Stop ends the display and everything it started, and removes its folder.
func (v *Virtual) Stop() {
	if v == nil {
		return
	}

	for _, c := range v.stops {
		if c.Process != nil {
			// Ask it to stop, the way this system asks. See sandbox.
			sandbox.StopGroup(c.Process.Pid)

			done := make(chan struct{})

			go func() { c.Wait(); close(done) }()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				sandbox.KillGroup(c.Process.Pid)
				<-done
			}
		}
	}

	v.stops = nil

	if v.runtime != "" {
		os.RemoveAll(v.runtime)
	}
}

// base is the environment the display's own programs run with: nothing of
// this program's, and a runtime folder of their own.
func base(runtime string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
		"LANG=C.UTF-8", "XDG_RUNTIME_DIR=" + runtime}
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	return lines[len(lines)-1]
}
