package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

/*
 * The few things that are always put to the owner.
 *
 * Freedom decides how much the assistant may do without asking, and on "never
 * stop" the answer is everything — its owner chose that, and it holds for
 * files, commands and mail. It does not hold for five things, because each of
 * them is its owner's in a way a file is not: putting software on the
 * machine, switching on an integration that runs as its own program or talks
 * to somebody else's server, connecting an account, spending money, and
 * writing a permanent member of the organisation who will act in its name
 * from then on. Each changes what the assistant is, rather than what it did
 * this afternoon.
 *
 * So a tool that does one of those says so, per call, and the loop asks
 * whatever the setting — the one place a standing yes is not enough. Refusals
 * still come first: a tool its owner has refused stays refused.
 */
type Consenting interface {
	// Consent is why this call must be put to the owner whatever has been
	// allowed, in a few words, or empty when this call is not one of those.
	Consent(args json.RawMessage) string
}

// ConsentFor is a call's reason to be asked about, or empty.
func ConsentFor(t Tool, args json.RawMessage) string {
	if c, ok := t.(Consenting); ok {
		return c.Consent(args)
	}

	return ""
}

/*
 * What else is always asked, after an audit found it was not: on "never stop"
 * a message went out, a model downloaded and a folder was deleted with nobody
 * asked. Each of these is its owner's the way installing is — something said
 * in their name, something put on or taken off their machine, something that
 * cannot be put back.
 */

func (SendEmail) Consent(json.RawMessage) string { return "sending a message in your name" }

func (InstallPart) Consent(json.RawMessage) string { return "installing software" }

func (InstallModel) Consent(json.RawMessage) string { return "downloading a model onto this machine" }

func (RemovePart) Consent(json.RawMessage) string { return "removing software from this machine" }

// Consent is asked of a command that cannot be taken back: see Destructive.
func (RunCommand) Consent(raw json.RawMessage) string {
	var a commandArgs

	json.Unmarshal(raw, &a)

	return Destructive(a.Argv)
}

/*
 * Destructive is why a command cannot be undone, or empty.
 *
 * Not every command that changes something — those ask or not as their owner
 * has said — but the ones whose change is gone for good: deleting, rewriting
 * history, overwriting a disk, stopping programs, doing any of it as root.
 * "Put it back" can restore a file this program wrote; it cannot restore one
 * rm removed.
 */
func Destructive(argv []string) string {
	if len(argv) == 0 {
		return ""
	}

	program := filepath.Base(argv[0])
	args := argv[1:]

	has := func(flags ...string) bool {
		for _, a := range args {
			for _, f := range flags {
				if a == f {
					return true
				}

				// Bundled short flags: -rf holds -r.
				if len(f) == 2 && f[0] == '-' && f[1] != '-' && strings.HasPrefix(a, "-") &&
					!strings.HasPrefix(a, "--") && strings.ContainsRune(a[1:], rune(f[1])) {
					return true
				}
			}
		}

		return false
	}

	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	switch program {
	case "sudo", "pkexec", "su", "doas":
		return "it runs as the administrator"
	case "rm", "unlink", "shred", "srm":
		return "it deletes files for good"
	case "dd", "wipefs", "fdisk", "parted", "sgdisk", "truncate", "blkdiscard":
		return "it overwrites data in place"
	case "kill", "pkill", "killall":
		return "it stops programs"
	case "find":
		if has("-delete") || strings.Contains(strings.Join(args, " "), "-exec rm") {
			return "it deletes files for good"
		}
	case "chmod", "chown", "chgrp":
		if has("-R", "--recursive") {
			return "it changes permissions on a whole tree"
		}
	case "systemctl":
		switch sub {
		case "stop", "disable", "mask", "kill", "poweroff", "reboot", "halt":
			return "it stops or disables a service"
		}
	case "shutdown", "reboot", "poweroff", "halt":
		return "it turns the machine off"
	case "git":
		for i, a := range args {
			if a == "-C" || a == "-c" {
				continue
			}

			if i > 0 && (args[i-1] == "-C" || args[i-1] == "-c") {
				continue
			}

			sub = a

			break
		}

		switch {
		case sub == "reset" && has("--hard"):
			return "it throws away uncommitted work"
		case sub == "clean" && has("-f", "--force"):
			return "it deletes untracked files for good"
		case sub == "push" && has("-f", "--force", "--force-with-lease", "--delete", "-d"):
			return "it rewrites or deletes history somewhere else"
		case sub == "branch" && has("-D"):
			return "it deletes a branch that may not be merged"
		case sub == "stash" && len(args) > 1 && (args[1] == "drop" || args[1] == "clear"):
			return "it throws away stashed work"
		case sub == "checkout" && has("--", "-f", "--force"), sub == "restore" && !has("--staged"):
			return "it throws away uncommitted changes"
		case sub == "filter-branch", sub == "filter-repo":
			return "it rewrites history"
		}
	}

	if strings.HasPrefix(program, "mkfs") {
		return "it formats a disk"
	}

	return ""
}
