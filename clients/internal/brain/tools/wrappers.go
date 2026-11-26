package tools

import (
	"encoding/base64"
	"strings"
	"unicode/utf16"
)

/*
 * More wrappers a command is read through (ASSISTANT-PROGRAM-USE-NOTES-1121,
 * 6): before these, a "never" on a program did not reach it when it was run
 * by one of them.
 *
 * Two kinds. Those that hand over a line to be run (su -c, runuser -c,
 * flock -c, watch, a terminal's -e "...", cmd /c, powershell -Command)
 * are read by handedLine, so every command of the line is reached (lineOf,
 * alsoRun). Those that run the words after their own (taskset, chrt,
 * flock, strace, ltrace, watch -x, unshare, nsenter, chroot,
 * dbus-run-session, xvfb-run, runuser -u, find -exec, a terminal's -x or
 * "--", powershell -File) are read by moreWrapped. Either way the program
 * is named so its card can refuse it, and no card allows it (Wrapped).
 */

// wrapperBase is a program's name as these wrappers are known: the last
// part of its place by either slash, without ".exe", in small letters
// for the Windows ones (CMD.EXE is cmd).
func wrapperBase(program string) string {
	base := program
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}

	if lower := strings.ToLower(base); strings.HasSuffix(lower, ".exe") {
		return strings.TrimSuffix(lower, ".exe")
	}

	switch lower := strings.ToLower(base); lower {
	case "cmd", "powershell", "pwsh":
		return lower
	}

	return base
}

/*
 * handedLine is the line a wrapper hands over to be run, whether there is
 * one, and whether argv starts with one of these wrappers at all (known):
 * when it does not, lineOf goes on to the shells.
 */
func handedLine(argv []string) (line string, ok, known bool) {
	if len(argv) == 0 {
		return "", false, false
	}

	rest := argv[1:]

	switch base := wrapperBase(argv[0]); base {
	case "su", "runuser":
		line, ok = suLine(rest)
		return line, ok, true
	case "flock":
		if after, isLine := flockCommand(rest); isLine {
			return after[0], true, true
		}

		return "", false, true
	case "watch":
		rest, exec := watchCommand(rest)
		if exec || len(rest) == 0 {
			return "", false, true
		}

		// watch hands its words to sh -c, joined.
		return strings.Join(rest, " "), true, true
	case "cmd":
		line, ok = cmdLine(rest)
		return line, ok, true
	case "powershell", "pwsh":
		line, ok, _ = powershellLine(base, rest)
		return line, ok, true
	default:
		if terminals[base] {
			if after, isLine := terminalCommand(base, rest); isLine {
				return after[0], true, true
			}

			return "", false, true
		}
	}

	return "", false, false
}

/*
 * moreWrapped is what one of the wrappers that run words runs, whether
 * argv starts with one (was), and whether it is one of these at all
 * (known). A wrapper with nothing to run (taskset -p, su alone, chroot
 * with no command, which starts a shell) is wrapped around nothing: no
 * program is named, so nothing is borrowed either.
 */
func moreWrapped(argv []string) (inside []string, was, known bool) {
	rest := argv[1:]

	switch base := wrapperBase(argv[0]); base {
	case "su", "runuser":
		if base == "runuser" && hasOption(rest, "-u", "--user") {
			return pastOptions(rest, "ugGsw", "--user", "--group", "--supp-group", "--shell",
				"--whitelist-environment"), true, true
		}

		if _, ok := suLine(rest); ok {
			return nil, false, false // the line, read by lineOf
		}

		// A shell for another user: nothing named.
		return nil, true, true
	case "taskset":
		if hasOption(rest, "-p", "--pid") || shortLetter(rest, 'p') {
			return nil, true, true
		}

		rest = pastOptions(rest, "")
		if len(rest) > 0 {
			rest = rest[1:] // the mask or the list of processors
		}

		return rest, true, true
	case "chrt":
		if hasOption(rest, "-p", "--pid") || shortLetter(rest, 'p') {
			return nil, true, true
		}

		rest = pastOptions(rest, "TPD", "--sched-runtime", "--sched-period", "--sched-deadline")
		if len(rest) > 0 && strings.Trim(rest[0], "0123456789") == "" {
			rest = rest[1:] // the priority
		}

		return rest, true, true
	case "flock":
		after, isLine := flockCommand(rest)
		if isLine {
			return nil, false, false
		}

		return after, true, true
	case "strace":
		// Options whose value is optional (--quiet, --decode-fds) take it
		// after "=" only.
		return pastOptions(rest, "abeEIoOpPsSuUX", "--attach", "--output", "--signal", "--string-limit", "--user",
			"--env", "--trace", "--status", "--columns", "--summary-columns", "--summary-sort-by", "--inject",
			"--fault", "--trace-path", "--detach-on"), true, true
	case "ltrace":
		return pastOptions(rest, "aADeFlnopsuwx", "--align", "--indent", "--library", "--output", "--config"), true, true
	case "watch":
		rest, exec := watchCommand(rest)
		if !exec {
			return nil, false, false
		}

		return rest, true, true
	case "unshare":
		return pastOptions(rest, "SGRw", "--setuid", "--setgid", "--root", "--wd", "--map-user", "--map-group",
			"--map-users", "--map-groups"), true, true
	case "nsenter":
		return pastOptions(rest, "tSG", "--target", "--setuid", "--setgid"), true, true
	case "chroot":
		rest = pastOptions(rest, "", "--userspec", "--groups")
		if len(rest) > 0 {
			rest = rest[1:] // the new root
		}

		return rest, true, true
	case "dbus-run-session":
		return pastOptions(rest, "", "--config-file", "--dbus-daemon"), true, true
	case "xvfb-run":
		return pastOptions(rest, "efnpsw", "--error-file", "--auth-file", "--server-num", "--xauth-protocol",
			"--server-args", "--wait"), true, true
	case "find":
		for i, w := range rest {
			if w == "-exec" || w == "-execdir" || w == "-ok" || w == "-okdir" {
				cmd := rest[i+1:]

				for j, c := range cmd {
					if c == ";" || c == "+" {
						cmd = cmd[:j]

						break
					}
				}

				return cmd, true, true
			}
		}

		return nil, false, false
	case "cmd":
		return nil, false, false
	case "powershell", "pwsh":
		line, ok, file := powershellLine(base, rest)

		switch {
		case file != nil:
			return file, true, true
		case ok && line == "":
			// An encoded line that does not decode: nothing can be named.
			return nil, true, true
		}

		return nil, false, false
	default:
		if terminals[base] {
			if after, isLine := terminalCommand(base, rest); !isLine && after != nil {
				return after, true, true
			}
		}
	}

	return nil, false, false
}

// namedWrapper is whether argv starts with one of the wrappers read here,
// which a "never" on its own card must go on refusing.
func namedWrapper(argv []string) bool {
	if len(argv) == 0 {
		return false
	}

	if _, _, known := handedLine(argv); known {
		return true
	}

	_, _, known := moreWrapped(argv)

	return known
}

// hasOption is whether one of the options stands among the words before
// "--" (a long one also written with "=").
func hasOption(rest []string, names ...string) bool {
	for _, w := range rest {
		if w == "--" {
			return false
		}

		for _, n := range names {
			if w == n || (strings.HasPrefix(n, "--") && strings.HasPrefix(w, n+"=")) {
				return true
			}
		}
	}

	return false
}

// shortLetter is whether a short option letter stands among letters
// written together ("-ap") before the first word that is not an option.
func shortLetter(rest []string, letter byte) bool {
	for _, w := range rest {
		if !strings.HasPrefix(w, "-") || w == "--" {
			return false
		}

		if !strings.HasPrefix(w, "--") && strings.IndexByte(w[1:], letter) >= 0 {
			return true
		}
	}

	return false
}

/*
 * suLine is the line su (or runuser without -u) is given with -c or
 * --command, anywhere among its words (su reads its options wherever they
 * stand: "su - you -c gimp"), and letters together ending in c ("-lc").
 */
func suLine(rest []string) (string, bool) {
	for i := 0; i < len(rest); i++ {
		w := rest[i]

		switch {
		case w == "--":
			return "", false
		case w == "-c" || w == "--command" || w == "-C" || w == "--session-command":
			if i+1 < len(rest) {
				return rest[i+1], true
			}

			return "", false
		case strings.HasPrefix(w, "--command="):
			return strings.TrimPrefix(w, "--command="), true
		case strings.HasPrefix(w, "--session-command="):
			return strings.TrimPrefix(w, "--session-command="), true
		case w == "-s" || w == "--shell" || w == "-g" || w == "--group" || w == "-G" || w == "--supp-group" ||
			w == "-w" || w == "--whitelist-environment":
			i++
		case len(w) > 2 && w[0] == '-' && w[1] != '-' && strings.HasSuffix(w, "c"):
			if i+1 < len(rest) {
				return rest[i+1], true
			}

			return "", false
		}
	}

	return "", false
}

/*
 * flockCommand is what flock runs: after its options and the file to lock,
 * the words of a command, or a line after -c (isLine, the line first).
 * flock with only a file or a number locks and runs nothing (nil).
 */
func flockCommand(rest []string) (after []string, isLine bool) {
	rest = pastOptions(rest, "wE", "--wait", "--timeout", "--conflict-exit-code")
	if len(rest) < 2 {
		return nil, false
	}

	rest = rest[1:] // the file

	if rest[0] == "-c" || rest[0] == "--command" {
		if len(rest) > 1 {
			return rest[1:2], true
		}

		return nil, false
	}

	return rest, false
}

// watchCommand is watch's words after its own options, and whether -x
// (--exec) runs them as words rather than as a line given to sh -c.
func watchCommand(rest []string) ([]string, bool) {
	exec := false

	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		w := rest[0]
		rest = rest[1:]

		switch {
		case w == "--":
			return rest, exec
		case w == "--exec":
			exec = true
		case w == "--interval" || w == "--equexit":
			if len(rest) > 0 {
				rest = rest[1:]
			}
		case strings.HasPrefix(w, "--"):
		default:
			if strings.IndexByte(w[1:], 'x') >= 0 {
				exec = true
			}

			// -n and -q take the next word unless the value is in the word.
			if last := w[len(w)-1]; (last == 'n' || last == 'q') && len(rest) > 0 {
				rest = rest[1:]
			}
		}
	}

	return rest, exec
}

// cmdLine is the line cmd is given after /c (or /k, /r), any case, with
// its other switches before it.
func cmdLine(rest []string) (string, bool) {
	for i, w := range rest {
		if !strings.HasPrefix(w, "/") {
			return "", false
		}

		switch strings.ToLower(w) {
		case "/c", "/k", "/r":
			if i+1 < len(rest) {
				return strings.Join(rest[i+1:], " "), true
			}

			return "", false
		}
	}

	return "", false
}

/*
 * powershellLine is what PowerShell is given: a line after -Command (or
 * any short form of it, -c, -cwa), a line encoded after -EncodedCommand
 * (-e, -ec, -enc...; ok with an empty line when it does not decode), or a
 * script and its words after -File (file). Windows PowerShell takes words
 * after its options as a line; pwsh takes them as a script.
 */
func powershellLine(base string, rest []string) (line string, ok bool, file []string) {
	for i := 0; i < len(rest); i++ {
		w := rest[i]

		if !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "/") {
			if base == "powershell" {
				return strings.Join(rest[i:], " "), true, nil
			}

			return "", false, rest[i:]
		}

		name := strings.ToLower(strings.TrimLeft(w, "-/"))
		if j := strings.IndexByte(name, ':'); j >= 0 {
			name = name[:j]
		}

		switch {
		case name == "":
		case name == "cwa" || (len(name) > len("command") && strings.HasPrefix("commandwithargs", name)):
			return strings.Join(rest[i+1:], " "), true, nil
		case strings.HasPrefix("command", name):
			return strings.Join(rest[i+1:], " "), true, nil
		case name == "e" || name == "ec" || name == "en" || (len(name) >= 3 && strings.HasPrefix("encodedcommand", name)):
			if i+1 >= len(rest) {
				return "", true, nil
			}

			return decodedPowershell(rest[i+1]), true, nil
		case name == "f" || (len(name) >= 2 && strings.HasPrefix("file", name)):
			if i+1 < len(rest) {
				return "", false, rest[i+1:]
			}

			return "", false, nil
		case powershellValued(name):
			i++
		}
	}

	return "", false, nil
}

// powershellValued is an option of PowerShell's that takes the next word.
func powershellValued(name string) bool {
	switch name {
	case "ex", "ep", "w", "v", "o", "of", "if", "inp", "wd", "config", "settings", "ea":
		return true
	}

	for _, long := range []string{"executionpolicy", "windowstyle", "version", "psconsolefile", "configurationname",
		"inputformat", "outputformat", "workingdirectory", "settingsfile", "custompipename", "encodedarguments"} {
		if len(name) >= 3 && strings.HasPrefix(long, name) {
			return true
		}
	}

	return false
}

// decodedPowershell is an -EncodedCommand's line (base64 of UTF-16LE), or
// empty when it is not one.
func decodedPowershell(encoded string) string {
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(b)%2 != 0 {
		return ""
	}

	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}

	return string(utf16.Decode(units))
}

/*
 * The terminals and how each is given a command: after "--" or -x as
 * words for all; after -e as a line for those in lineTerminals, as words
 * for the rest (with one word holding a space read as a line, which is
 * how those run it). kitty and foot also take words after their options.
 */
var terminals = map[string]bool{
	"gnome-terminal": true, "gnome-terminal.wrapper": true, "xterm": true, "uxterm": true, "konsole": true,
	"xfce4-terminal": true, "x-terminal-emulator": true, "tilix": true, "alacritty": true, "terminator": true,
	"mate-terminal": true, "lxterminal": true, "urxvt": true, "rxvt": true, "st": true, "kgx": true,
	"ptyxis": true, "qterminal": true, "deepin-terminal": true, "sakura": true, "terminology": true,
	"wezterm": true, "kitty": true, "foot": true, "footclient": true, "guake": true, "tilda": true,
}

var lineTerminals = map[string]bool{
	"gnome-terminal": true, "gnome-terminal.wrapper": true, "xfce4-terminal": true, "mate-terminal": true,
	"terminator": true, "lxterminal": true, "tilix": true, "sakura": true, "qterminal": true,
	"deepin-terminal": true, "terminology": true, "guake": true, "tilda": true,
}

/*
 * terminalCommand is what a terminal is told to run: words (after "--",
 * -x, --execute, or -e for most), or a line (isLine, the line first). A
 * terminal told nothing runs only a shell of the person's (nil).
 */
func terminalCommand(base string, rest []string) (after []string, isLine bool) {
	for i, w := range rest {
		next := rest[i+1:]

		switch {
		case w == "--" || w == "-x" || w == "--execute":
			return next, false
		case w == "-e" || w == "--command":
			if len(next) == 0 {
				return nil, false
			}

			if lineTerminals[base] || (len(next) == 1 && strings.ContainsAny(next[0], " \t")) {
				return next[:1], true
			}

			return next, false
		case strings.HasPrefix(w, "--command=") || strings.HasPrefix(w, "--execute=") || strings.HasPrefix(w, "-e="):
			_, value, _ := strings.Cut(w, "=")

			return []string{value}, true
		}
	}

	switch base {
	case "kitty":
		if words := pastOptions(rest, "cdoT", "--class", "--name", "--title", "--config", "--override", "--directory",
			"--session", "--listen-on", "--start-as", "--instance-group", "--watcher", "--os-window-tag"); len(words) > 0 {
			return words, false
		}
	case "foot", "footclient":
		if words := pastOptions(rest, "ctTawWfDod", "--config", "--term", "--title", "--app-id", "--window-size-pixels",
			"--window-size-chars", "--font", "--working-directory", "--override", "--log-level", "--server-socket"); len(words) > 0 {
			return words, false
		}
	}

	return nil, false
}
