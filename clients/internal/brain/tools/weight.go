package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/risk"
)

/*
 * How serious one call is, as against whether it changes anything.
 *
 * Risk is the gate and says only that: Safe or Mutating. Most tools need
 * nothing more, and the level follows from it — looking is low, changing
 * something is medium. Weighed is for the few where the same tool is a small
 * thing or a very large one depending on what it was handed: run_command
 * listing a folder and run_command formatting a disk are one tool.
 *
 * Kept on the arguments rather than the tool's name, because the name is
 * exactly what does not tell them apart.
 */
type Weighed interface {
	Weight(args json.RawMessage) risk.Level
}

// LevelOf is how serious this call to this tool is.
func LevelOf(t Tool, args json.RawMessage) risk.Level {
	if w, ok := t.(Weighed); ok {
		return w.Weight(args)
	}

	if t.Risk() == Mutating {
		return risk.Medium
	}

	return risk.Low
}

/*
 * A command is weighed on what it runs.
 *
 * The first word, past sudo, and what it is being asked to do. Not a sandbox
 * and not a parser of every program there is — a short list of the commands
 * that cannot be undone, or that act as the whole machine, which is the
 * difference a person approving one needs pointed out to them.
 */
func (RunCommand) Weight(raw json.RawMessage) risk.Level {
	var a commandArgs

	if argsOf(raw, &a) != nil || len(a.Argv) == 0 {
		return risk.Medium
	}

	argv := a.Argv
	level := risk.Medium

	for len(argv) > 0 && (argv[0] == "sudo" || argv[0] == "pkexec" || argv[0] == "doas") {
		level = risk.High
		argv = argv[1:]
	}

	if len(argv) == 0 {
		return level
	}

	name := filepath.Base(argv[0])
	rest := " " + strings.Join(argv[1:], " ") + " "

	switch name {
	case "dd", "mkfs", "fdisk", "parted", "wipefs", "shred", "sgdisk":
		return risk.Critical
	case "rm":
		if strings.Contains(rest, " / ") || strings.Contains(rest, " ~ ") ||
			strings.Contains(rest, " /home ") || strings.Contains(rest, " --no-preserve-root ") {
			return risk.Critical
		}

		if strings.Contains(rest, " -r") || strings.Contains(rest, " -f") ||
			strings.Contains(rest, " --recursive") {
			return risk.Max(level, risk.High)
		}
	case "shutdown", "reboot", "poweroff", "halt", "systemctl", "service",
		"apt", "apt-get", "dpkg", "snap", "flatpak", "chmod", "chown", "useradd", "userdel",
		"passwd", "iptables", "ufw", "crontab":
		return risk.Max(level, risk.High)
	case "git":
		if strings.Contains(rest, " push ") && (strings.Contains(rest, " --force") ||
			strings.Contains(rest, " -f ")) {
			return risk.Max(level, risk.High)
		}

		if strings.Contains(rest, " reset --hard") || strings.Contains(rest, " clean -") {
			return risk.Max(level, risk.High)
		}
	}

	if strings.HasPrefix(name, "mkfs.") {
		return risk.Critical
	}

	return level
}

// A lock or an alarm is the physical world; a lamp is not.
func (SetDevice) Weight(raw json.RawMessage) risk.Level {
	var a deviceArgs

	argsOf(raw, &a)

	for _, serious := range []string{"lock.", "alarm_control_panel.", "cover.garage", "valve."} {
		if strings.HasPrefix(a.ID, serious) {
			return risk.High
		}
	}

	return risk.Medium
}

// Writing inside somebody's own files is medium; writing where the system
// keeps itself is not.
func (WriteFile) Weight(raw json.RawMessage) risk.Level {
	var a struct {
		Path string `json:"path"`
	}

	argsOf(raw, &a)

	return pathWeight(a.Path)
}

func (EditFile) Weight(raw json.RawMessage) risk.Level {
	var a editArgs

	argsOf(raw, &a)

	return pathWeight(a.Path)
}

func pathWeight(path string) risk.Level {
	clean := filepath.Clean(path)

	for _, system := range []string{"/etc", "/usr", "/boot", "/bin", "/sbin", "/lib", "/var/lib"} {
		if clean == system || strings.HasPrefix(clean, system+"/") {
			return risk.High
		}
	}

	if IsSensitive(clean) {
		return risk.High
	}

	return risk.Medium
}

// Software on and off the machine, some of it with a password prompt.
func (InstallPart) Weight(json.RawMessage) risk.Level  { return risk.High }
func (InstallModel) Weight(json.RawMessage) risk.Level { return risk.Medium }
func (RemovePart) Weight(json.RawMessage) risk.Level   { return risk.High }
