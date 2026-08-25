// Command pn-brain-doctor reports what PN Brain needs from this machine and can
// install the missing pieces from a terminal.
//
// The desktop app does the same job in a window, sharing the same preflight
// package. This exists for headless machines, for servers, and for diagnosing
// a setup where the window itself will not open.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"pn-brain/internal/preflight"
)

const (
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	dim    = "\033[2m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

func colourFor(r preflight.Result) string {
	switch {
	case r.Satisfied():
		return green
	case r.Requirement.Optional:
		return yellow
	default:
		return red
	}
}

func markFor(r preflight.Result) string {
	switch {
	case r.Satisfied():
		return "✓"
	case r.Requirement.Optional:
		return "!"
	default:
		return "✗"
	}
}

// reportMachine states what the machine can actually do, because the honest
// answer to "which model should I use?" depends entirely on it.
func reportMachine() {
	hw := preflight.DetectHardware()
	gpu := "no discrete GPU"

	if hw.HasGPU {
		gpu = "GPU: " + hw.GPUName
	}

	fmt.Printf("  %s%d cores  ·  %dGB RAM  ·  %s%s\n", dim, hw.CPUCores, hw.RAMGB, gpu, reset)

	choice := preflight.RecommendModel(hw)

	if preflight.CanRunLocalModels(hw) {
		fmt.Printf("  %ssuggested local model: %s (%s) — %s%s\n\n",
			dim, choice.Model, choice.SizeNote, choice.SpeedNote, reset)

		return
	}

	fmt.Printf("  %stoo little memory for a local model to be pleasant; an API key is the better route%s\n\n", yellow, reset)
}

func report(results []preflight.Result) (blocking, fixable int) {
	fmt.Printf("\n%sPN Brain — system check%s\n\n", bold, reset)
	reportMachine()

	for _, r := range results {
		fmt.Printf("  %s%s%s  %-22s %s%s%s\n",
			colourFor(r), markFor(r), reset,
			r.Requirement.Name,
			dim, preflight.Describe(r.State, r.Detail), reset,
		)

		if r.Satisfied() {
			continue
		}

		fmt.Printf("     %s%s%s\n", dim, r.Requirement.Consequence, reset)

		if !r.Requirement.Installable() && r.Requirement.ManualHint != "" {
			fmt.Printf("     %s→ %s%s\n", yellow, r.Requirement.ManualHint, reset)
		}

		if r.Blocking() {
			blocking++
		}

		if r.Requirement.Installable() {
			fixable++
		}
	}

	return blocking, fixable
}

func confirm(prompt string) bool {
	fmt.Printf("\n%s [y/N]: ", prompt)

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}

	answer = strings.ToLower(strings.TrimSpace(answer))

	return answer == "y" || answer == "yes"
}

func main() {
	autoInstall := flag.Bool("install", false, "Install what is missing, asking first")
	assumeYes := flag.Bool("yes", false, "With --install, don't ask")
	quiet := flag.Bool("quiet", false, "Print nothing when everything is fine")
	flag.Parse()

	results := preflight.Check()

	if preflight.BlockingCount(results) == 0 && *quiet && !*autoInstall {
		return
	}

	blocking, fixable := report(results)

	if blocking == 0 {
		fmt.Printf("\n%s✓ Ready.%s\n\n", green, reset)
	} else {
		fmt.Printf("\n%s%d requirement(s) must be met before PN Brain can run.%s\n", red, blocking, reset)
	}

	if !*autoInstall {
		if fixable > 0 {
			fmt.Printf("%sRun 'pn-brain-doctor --install' to fix %d of them automatically.%s\n\n", dim, fixable, reset)
		}

		if blocking > 0 {
			os.Exit(1)
		}

		return
	}

	if fixable == 0 {
		fmt.Printf("%sNothing here can be installed automatically.%s\n\n", dim, reset)

		if blocking > 0 {
			os.Exit(1)
		}

		return
	}

	for _, r := range results {
		if r.Satisfied() || !r.Requirement.Installable() {
			continue
		}

		if !*assumeYes && !confirm(fmt.Sprintf("Install %s? (%s)", r.Requirement.Name, r.Requirement.Why)) {
			fmt.Printf("%s  skipped%s\n", dim, reset)

			continue
		}

		fmt.Printf("\n%s→ %s%s\n", bold, r.Requirement.Name, reset)

		if err := preflight.Install(r.Requirement, os.Stdout); err != nil {
			fmt.Printf("%s  failed: %v%s\n", red, err, reset)
		}
	}

	// Re-check rather than trusting exit codes: apt can succeed while leaving
	// the requirement still unmet, and reporting success on that would be a lie.
	fmt.Printf("\n%sRe-checking…%s\n", dim, reset)

	if blockingAfter, _ := report(preflight.Check()); blockingAfter > 0 {
		fmt.Printf("\n%s%d still unmet.%s\n\n", red, blockingAfter, reset)
		os.Exit(1)
	}

	fmt.Printf("\n%s✓ Ready.%s\n\n", green, reset)
}
