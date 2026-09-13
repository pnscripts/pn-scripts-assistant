package tunnel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

/*
 * Bringing the tunnel up, which genuinely needs root.
 *
 * Creating a network interface is a thing only the system may do, and
 * pretending otherwise would mean either a program that quietly fails or one
 * that asks to run as root all the time. So this asks for a password once,
 * through the same graphical prompt the installer uses — and says exactly what
 * it is about to run, because "give me your password" with no sentence after
 * it is how people learn to type it without reading.
 */

// Where wg-quick looks. Not configurable: this is the path the tool is written
// against, and a tunnel in a folder it does not read is a tunnel that does not
// exist.
const systemFolder = "/etc/wireguard"

// Installed reports whether the tools to run a tunnel are here at all.
func Installed() bool {
	_, err := exec.LookPath("wg-quick")

	return err == nil
}

// Up reports whether the tunnel is running right now.
func Up() bool {
	// The interface existing is the whole question, and asking the kernel
	// needs no privilege — unlike `wg show`, which does.
	_, err := os.Stat(filepath.Join("/sys/class/net", Interface))

	return err == nil
}

/*
 * Command is what will be run, so it can be shown before it is.
 *
 * Written out rather than described. Somebody about to type their password
 * should be able to read the thing they are agreeing to, and "set up the
 * tunnel" is not that.
 */
func Command(root string) []string {
	return []string{
		"sh", "-c",
		fmt.Sprintf("install -d -m 700 %s && install -m 600 %s %s && wg-quick down %s 2>/dev/null; wg-quick up %s",
			systemFolder,
			shellSafe(filepath.Join(root, Interface+".conf")),
			shellSafe(filepath.Join(systemFolder, Interface+".conf")),
			Interface, Interface),
	}
}

/*
 * Start writes the configuration and brings the tunnel up.
 *
 * Written to the brain's folder first and copied by the privileged half, so
 * the only thing running as root is one command somebody has read — rather
 * than this program holding a root file handle for the rest of the session.
 */
func Start(ctx context.Context, root string, t *Tunnel, ask func(argv []string) ([]byte, error)) error {
	if !Installed() {
		return fmt.Errorf(
			"WireGuard is not installed yet — it is in the list of parts this machine " +
				"needs, under What it runs on")
	}

	if why := t.Ready(); why != "" {
		return fmt.Errorf("%s", why)
	}

	path := filepath.Join(root, Interface+".conf")

	// The private key of the house. Nobody but its owner reads this.
	if err := os.WriteFile(path, []byte(t.MachineConfig()), 0o600); err != nil {
		return fmt.Errorf("writing the tunnel's configuration: %w", err)
	}

	output, err := ask(Command(root))
	if err != nil {
		return fmt.Errorf("%s", plainly(string(output), err))
	}

	// Asked rather than assumed: wg-quick can report success and leave
	// nothing behind if the kernel module will not load.
	for i := 0; i < 20; i++ {
		if Up() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	return fmt.Errorf("the tunnel was set up but no interface appeared")
}

// Stop takes it down again.
func Stop(ask func(argv []string) ([]byte, error)) error {
	if !Up() {
		return nil
	}

	output, err := ask([]string{"sh", "-c", "wg-quick down " + Interface})
	if err != nil {
		return fmt.Errorf("%s", plainly(string(output), err))
	}

	return nil
}

/*
 * plainly turns whatever the tools said into something worth reading.
 *
 * wg-quick is a shell script and reports through whichever of ip, wg or
 * resolvconf failed, so its last line is usually the useful one and its first
 * is usually not.
 */
func plainly(output string, err error) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])

		switch {
		case line == "":
			continue
		case strings.Contains(line, "Operation not permitted"):
			return "the password was refused, so nothing was changed"
		case strings.Contains(line, "not found"), strings.Contains(line, "No such file"):
			return "part of WireGuard is missing: " + line
		case strings.HasPrefix(line, "["), strings.HasPrefix(line, "$"):
			// wg-quick echoes each command it runs; those are not the error.
			continue
		default:
			return line
		}
	}

	return err.Error()
}

// shellSafe quotes a path for the one shell command this runs. Paths here come
// from the brain's own root, which is chosen by its owner and can contain
// anything a filename can.
func shellSafe(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
