package orchestrator

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"pn-scripts-assistant/internal/brain/display"
	"pn-scripts-assistant/internal/brain/sandbox"
)

// Machine is what this computer is and can do, read rather than assumed.
type Machine struct {
	OS     string `json:"os"`
	Kernel string `json:"kernel"`
	Arch   string `json:"arch"`
	CPU    string `json:"cpu"`
	Cores  int    `json:"cores"`

	RAM     uint64 `json:"ram_bytes"`
	RAMFree uint64 `json:"ram_available_bytes"`

	GPU []string `json:"gpu,omitempty"`

	Disks []Disk `json:"disks"`

	// Display is the screen the owner is using; Virtual is whether a private
	// one can be made for pictures, and how or why not.
	Display string `json:"display"`
	Virtual string `json:"virtual_display"`

	// Network is whether a way out exists. Nothing is contacted to find out.
	Network string `json:"network"`

	// Landlock is the kernel confinement available to commands, 0 for none.
	Landlock int `json:"landlock_abi"`
}

// Disk is free space where something would be put.
type Disk struct {
	For  string `json:"for"`
	Path string `json:"path"`
	Free int64  `json:"free_bytes"`
}

// ReadMachine reads this machine, putting down where the brain and the
// models live as the disks worth knowing about.
func ReadMachine(brainRoot string) Machine {
	m := Machine{Arch: runtime.GOARCH, Cores: runtime.NumCPU()}

	m.OS = osName()

	// Which kernel, where the system has a way of saying. See kernel_linux.go.
	m.Kernel = kernelName()

	m.CPU = cpuModel()
	m.RAM, m.RAMFree = memory()
	m.GPU = gpus()

	home, _ := os.UserHomeDir()

	for _, d := range []struct{ what, path string }{
		{"home and local models", filepath.Join(home, ".ollama")},
		{"the brain", brainRoot},
		{"temporary files", os.TempDir()},
	} {
		if free, ok := freeAt(d.path); ok {
			m.Disks = append(m.Disks, Disk{For: d.what, Path: d.path, Free: free})
		}
	}

	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		m.Display = "wayland " + os.Getenv("WAYLAND_DISPLAY")
	case os.Getenv("DISPLAY") != "":
		m.Display = "x11 " + os.Getenv("DISPLAY")
	default:
		m.Display = "none"
	}

	if kind, err := display.Available(); err == nil {
		m.Virtual = string(kind)
	} else {
		m.Virtual = "none: " + err.Error()
	}

	m.Network = network()

	if abi, err := sandbox.ABI(); err == nil {
		m.Landlock = abi
	}

	return m
}

// FreeFor is the free space where local models are kept.
func (m Machine) FreeFor(what string) int64 {
	for _, d := range m.Disks {
		if strings.HasPrefix(d.For, what) {
			return d.Free
		}
	}

	return -1
}

func osName() string {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}

	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}

	return runtime.GOOS
}

func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}

	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if name, value, ok := strings.Cut(s.Text(), ":"); ok && strings.TrimSpace(name) == "model name" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func memory() (total, available uint64) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}

	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		kb, _ := strconv.ParseUint(fields[1], 10, 64)

		switch fields[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			available = kb * 1024
		}
	}

	return total, available
}

// gpus is the graphics devices by name, from lspci when it is here.
func gpus() []string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return nil
	}

	var found []string

	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)

		if strings.Contains(lower, "vga compatible") || strings.Contains(lower, "3d controller") ||
			strings.Contains(lower, "display controller") {
			if _, name, ok := strings.Cut(line, ": "); ok {
				found = append(found, strings.TrimSpace(name))
			}
		}
	}

	return found
}

// network is whether there is a default route: a way out, without asking
// anybody on the other side.
func network() string {
	raw, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "unknown"
	}

	for _, line := range strings.Split(string(raw), "\n")[1:] {
		fields := strings.Fields(line)

		if len(fields) > 1 && fields[1] == "00000000" {
			return "a route out exists (" + fields[0] + "); nothing was contacted to check it"
		}
	}

	return "no route out"
}
