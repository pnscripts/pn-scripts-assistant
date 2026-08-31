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
		// Not the smallest model that fits, but the smallest that uses tools
		// sensibly. Given tool definitions and asked to say a word, llama3.2:3b
		// called write_file instead; asked for arithmetic it emitted JSON naming
		// a tool that does not exist. A model that mishandles tools is worse
		// than no tools at all, and the size difference costs little here.
		return ModelChoice{
			Model:     "qwen2.5-coder:7b",
			SizeNote:  "~4.7GB",
			SpeedNote: "usable on CPU — several seconds per reply",
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

/*
 * Offering a few models rather than one.
 *
 * RecommendModel answers "what should I install" with a single name, and that
 * is the right answer for somebody who has no way to judge between eight of
 * them. But it is the only answer they were given, and the trade it makes on
 * their behalf — a larger model that answers well and slowly — is exactly the
 * one people differ on. Somebody who wants a reply in two seconds and somebody
 * who wants the best answer this machine can produce are both served badly by
 * a single choice made for them.
 *
 * So: three, in the order they matter, with the recommendation still marked.
 * Three is few enough to read at a glance and wide enough to cover the
 * disagreement. Anything more is the list that made a single suggestion the
 * better design in the first place.
 */

// ModelOption is one model somebody could install, and why they might.
type ModelOption struct {
	ModelChoice

	// Label is what the difference is, in a word: what somebody is choosing
	// between rather than what they are getting.
	Label string

	// Recommended marks the one RecommendModel would have picked alone.
	Recommended bool
}

/*
 * ModelOptions is what to offer this machine, best first.
 *
 * The recommendation keeps its place in the order rather than being lifted to
 * the top: on a machine where the sensible default is the middle one, showing
 * it first would hide that something faster exists, which is the choice most
 * people actually want to make.
 */
func ModelOptions(hw Hardware) []ModelOption {
	recommended := RecommendModel(hw)

	var options []ModelOption

	add := func(label, model, size, speed string) {
		options = append(options, ModelOption{
			ModelChoice: ModelChoice{Model: model, SizeNote: size, SpeedNote: speed},
			Label:       label,
			Recommended: model == recommended.Model,
		})
	}

	/*
	 * The quick one is offered on every machine.
	 *
	 * It used to be avoided because it mishandled tools — asked to say a word
	 * it called write_file instead. That was true of the model and is now
	 * handled before the model sees anything: conversation is offered no tools
	 * at all, and the ones that change the machine have to be asked for. The
	 * reason for hiding it has gone, and on a processor the difference between
	 * three billion parameters and seven is the difference between a reply and
	 * a wait.
	 */
	add("Quickest", "llama3.2:3b", "~2GB", "answers in a few seconds on a processor")

	if hw.RAMGB >= 16 {
		add("Balanced", "qwen2.5-coder:7b", "~4.7GB",
			"slower, and noticeably better at using tools")
	}

	if hw.RAMGB >= 16 && hw.HasGPU {
		add("Best here", "qwen2.5:7b", "~4.7GB", "fast on your graphics card")
	}

	/*
	 * A machine too small for any of the above still gets an answer.
	 *
	 * Offering nothing would be the honest reading of "this will disappoint
	 * you", and it is the wrong one: somebody on a small machine would rather
	 * have a slow assistant than a page telling them they cannot.
	 */
	if len(options) == 0 || hw.RAMGB < 8 {
		add("Smallest", "llama3.2:1b", "~1.3GB",
			"the only size that fits comfortably here")
	}

	/*
	 * And if the recommendation is not among them, it is added.
	 *
	 * A list that omits the model the rest of the program would have chosen is
	 * a list that disagrees with itself, and the disagreement would only show
	 * up on hardware nobody tested.
	 */
	var has bool

	for _, o := range options {
		if o.Recommended {
			has = true
		}
	}

	if !has {
		options = append(options, ModelOption{
			ModelChoice: recommended,
			Label:       "Suggested",
			Recommended: true,
		})
	}

	return options
}
