package preflight

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Hardware is what the machine can realistically run.
//
// This exists because the honest answer to "which model should I use?" is
// entirely determined by it. Recommending an 8B model to someone with 8GB of
// RAM produces an assistant that swaps to death, and recommending a 1B model to
// someone with a 24GB GPU wastes what they have.
type Hardware struct {
	CPUCores int
	RAMGB    int
	HasGPU   bool
	GPUName  string
}

func DetectHardware() Hardware {
	hw := Hardware{CPUCores: runtime.NumCPU()}
	hw.RAMGB = detectRAMGB()
	hw.HasGPU, hw.GPUName = detectGPU()

	return hw
}

func detectRAMGB() int {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}

		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "MemTotal:") {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0
			}

			kb, err := strconv.Atoi(fields[1])
			if err != nil {
				return 0
			}

			return kb / 1024 / 1024
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}

		bytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}

		return int(bytes / 1024 / 1024 / 1024)
	}

	return 0
}

// detectGPU looks for a discrete GPU with usable acceleration. Integrated
// graphics are deliberately not counted: they share system memory and give
// almost nothing for language model inference, so treating them as a GPU would
// produce recommendations the machine cannot honour.
func detectGPU() (bool, string) {
	if name := versionOf("nvidia-smi", "--query-gpu=name", "--format=csv,noheader"); name != "" {
		return true, name
	}

	if runtime.GOOS == "darwin" {
		// Apple Silicon shares memory between CPU and GPU, and Ollama uses it
		// well, so it counts.
		if arch := versionOf("uname", "-m"); strings.Contains(arch, "arm64") {
			return true, "Apple Silicon"
		}
	}

	if commandExists("rocm-smi") {
		return true, "AMD ROCm"
	}

	return false, ""
}

// ModelChoice is a recommendation with the reasoning attached, because the
// numbers alone ("7B") mean nothing to most people.
type ModelChoice struct {
	Model     string
	SizeNote  string
	SpeedNote string
}

// recommendModel picks the largest model the machine can run without becoming
// unpleasant. The limits are conservative on purpose: a model that technically
// loads but takes two minutes per reply is worse than a smaller one that
// answers, and someone evaluating a new assistant will judge it on the first
// exchange.
func RecommendModel(hw Hardware) ModelChoice {
	switch {
	case hw.RAMGB >= 16 && hw.HasGPU:
		return ModelChoice{
			Model:     "qwen2.5:7b",
			SizeNote:  "~4.7GB",
			SpeedNote: "fast on your GPU",
		}
	case hw.RAMGB >= 16:
		return ModelChoice{
			Model:     "llama3.2:3b",
			SizeNote:  "~2GB",
			SpeedNote: "usable on CPU — expect a few seconds per reply",
		}
	case hw.RAMGB >= 8:
		return ModelChoice{
			Model:     "llama3.2:3b",
			SizeNote:  "~2GB",
			SpeedNote: "slow on CPU, but workable",
		}
	default:
		return ModelChoice{
			Model:     "llama3.2:1b",
			SizeNote:  "~1.3GB",
			SpeedNote: "the only size that fits comfortably here",
		}
	}
}

// canRunLocalModels reports whether local inference is worth suggesting at all.
// Below this, the honest advice is to use an API rather than to sell someone a
// local setup that will disappoint them.
func CanRunLocalModels(hw Hardware) bool {
	return hw.RAMGB >= 6
}
