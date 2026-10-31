package tools

import (
	"encoding/json"
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
	return destructive(argv, 0)
}

func destructive(argv []string, depth int) string {
	if len(argv) == 0 || depth > 4 {
		return ""
	}

	/*
	 * A shell handed its program as a string is judged by that program.
	 *
	 * "bash -c 'rm -rf ~/Documents'" used to read as bash, which deletes
	 * nothing; and on Windows every deletion is a command of cmd or
	 * PowerShell, never a program of its own. Found by the 1.0 security
	 * review. An encoded PowerShell command cannot be read at all, so it is
	 * asked about as what it might be.
	 */
	if commands, hidden, ok := Inline(argv); ok {
		if hidden {
			return HiddenCommand
		}

		for _, command := range commands {
			if why := destructive(command, depth+1); why != "" {
				return why
			}
		}

		return ""
	}

	program := ProgramName(argv[0])
	args := argv[1:]

	// Programs that run the rest of their arguments as a command.
	switch program {
	case "xargs", "env", "nohup", "nice", "timeout", "time", "exec", "command", "builtin", "stdbuf", "ionice":
		rest := args
		for len(rest) > 0 && (strings.HasPrefix(rest[0], "-") || strings.Contains(rest[0], "=") ||
			(program == "timeout" && len(rest[0]) > 0 && rest[0][0] >= '0' && rest[0][0] <= '9')) {
			rest = rest[1:]
		}

		return destructive(rest, depth+1)
	}

	if why := windowsDestructive(program, args); why != "" {
		return why
	}

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

// HiddenCommand is why a command whose program cannot be read is asked about.
const HiddenCommand = "it runs a hidden command that cannot be read, so it may change the whole system"

/*
 * ProgramName is a program's own name, the same on every system: no folder
 * (either slash), no .exe, lower case — "C:\\Windows\\System32\\cmd.exe" is cmd.
 */
func ProgramName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}

	path = strings.ToLower(path)

	for _, ext := range []string{".exe", ".com", ".bat", ".cmd"} {
		path = strings.TrimSuffix(path, ext)
	}

	return path
}

/*
 * Inline is the program a shell was handed as a string, split into the
 * commands the shell would run: sh/bash/zsh -c, cmd /c, powershell -Command
 * (or no flag at all, which is the same thing to PowerShell). hidden is an
 * encoded PowerShell command, which cannot be read. ok is false when argv is
 * not such a call.
 *
 * The split is deliberately generous — on ; & | newlines, $( ` ( ) { } — so a
 * command hidden in a substitution or a script block is still seen. Quotes are
 * dropped rather than parsed: the question is only which programs appear.
 */
func Inline(argv []string) (commands [][]string, hidden bool, ok bool) {
	if len(argv) < 2 {
		return nil, false, false
	}

	program := ProgramName(argv[0])
	args := argv[1:]

	var script string

	switch program {
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "ash", "busybox":
		for i, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsRune(a[1:], 'c') && i+1 < len(args) {
				script = args[i+1]
				ok = true

				break
			}
		}
	case "cmd":
		for i, a := range args {
			if l := strings.ToLower(a); l == "/c" || l == "/k" || l == "/r" {
				script = strings.Join(args[i+1:], " ")
				ok = true

				break
			}
		}
	case "powershell", "pwsh":
		for i := 0; i < len(args); i++ {
			l := strings.ToLower(args[i])

			switch {
			case l == "-e" || l == "-ec" || strings.HasPrefix(l, "-enc"):
				return nil, true, true
			case l == "-c" || strings.HasPrefix(l, "-com"):
				script = strings.Join(args[i+1:], " ")
				ok = true
			case l == "-f" || strings.HasPrefix(l, "-fi"):
				// A script file: run like any program, judged like one.
				return nil, false, false
			case strings.HasPrefix(l, "-"):
				// -NoProfile, -ExecutionPolicy Bypass and the like.
				if l == "-executionpolicy" || l == "-ep" || l == "-windowstyle" || l == "-w" ||
					strings.HasPrefix(l, "-config") || l == "-version" || l == "-v" || l == "-wd" {
					i++
				}

				continue
			default:
				// No flag: the rest is the command.
				script = strings.Join(args[i:], " ")
				ok = true
			}

			break
		}
	}

	if !ok {
		return nil, false, false
	}

	split := strings.NewReplacer(";", "\n", "&", "\n", "|", "\n", "$(", "\n", "`", "\n",
		"(", "\n", ")", "\n", "{", "\n", "}", "\n", "\r", "\n")

	for _, line := range strings.Split(split.Replace(script), "\n") {
		var words []string

		for _, word := range strings.Fields(line) {
			if word = strings.Trim(word, `'"`); word != "" {
				words = append(words, word)
			}
		}

		if len(words) > 0 {
			commands = append(commands, words)
		}
	}

	return commands, false, true
}

/*
 * windowsDestructive is the same question for cmd and PowerShell, whose
 * deleting, formatting and stopping are commands rather than programs.
 * PowerShell's aliases (del, rd, ri, erase, rm, rmdir) all mean Remove-Item.
 */
func windowsDestructive(program string, args []string) string {
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	switch program {
	case "del", "erase", "rd", "rmdir", "remove-item", "ri", "clear-content", "clc":
		return "it deletes files for good"
	case "format", "format-volume", "clear-disk", "diskpart", "initialize-disk", "remove-partition":
		return "it formats a disk"
	case "taskkill", "tskill", "stop-process", "spps":
		return "it stops programs"
	case "stop-service", "spsv", "set-service", "remove-service":
		return "it stops or disables a service"
	case "sc":
		switch sub {
		case "stop", "delete", "config":
			return "it stops or disables a service"
		}
	case "net":
		switch sub {
		case "stop", "user", "localgroup":
			return "it stops a service or changes accounts — a change to the whole system"
		}
	case "stop-computer", "restart-computer":
		return "it turns the machine off"
	case "runas":
		return "it runs as the administrator"
	case "start-process", "saps", "start":
		for _, a := range args {
			if strings.EqualFold(a, "runas") {
				return "it runs as the administrator"
			}
		}
	case "reg":
		switch sub {
		case "delete", "add", "import", "restore":
			return "it changes the whole system's registry"
		}
	case "remove-itemproperty", "set-itemproperty", "bcdedit", "set-executionpolicy", "takeown", "icacls", "cacls":
		return "it changes the whole system's settings or permissions"
	}

	return ""
}
