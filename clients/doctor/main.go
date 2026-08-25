// Command pnexus-doctor checks what Pnexus needs from this machine, and can
// install the missing pieces.
//
// It is written to be run repeatedly rather than once. Requirements drift:
// models get replaced, Docker gets upgraded, a distribution moves a library.
// So this reports current state every time, and installing is just "make the
// state match" — the same command works on a brand-new machine and on one that
// has been running Pnexus for a year.
//
// It lives outside the brain on purpose. Something has to be able to say "you
// have no Docker" on a computer where the brain therefore cannot start, so this
// is a small static Go binary with no dependencies of its own.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	dim    = "\033[2m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

type result struct {
	req    Requirement
	state  State
	detail string
}

func fileGlobExists(pattern string) bool {
	matches, err := filepath.Glob(pattern)

	return err == nil && len(matches) > 0
}

func runChecks() []result {
	results := make([]result, 0, len(requirements()))

	for _, req := range requirements() {
		state, detail := req.Check()
		results = append(results, result{req: req, state: state, detail: detail})
	}

	return results
}

func colourFor(r result) string {
	switch {
	case r.state == OK:
		return green
	case r.req.Optional:
		return yellow
	default:
		return red
	}
}

func markFor(r result) string {
	switch {
	case r.state == OK:
		return "✓"
	case r.req.Optional:
		return "!"
	default:
		return "✗"
	}
}

func report(results []result) (blocking, fixable int) {
	fmt.Printf("\n%sPnexus — system check%s\n\n", bold, reset)

	for _, r := range results {
		fmt.Printf("  %s%s%s  %-22s %s%s%s\n",
			colourFor(r), markFor(r), reset,
			r.req.Name,
			dim, describe(r.state, r.detail), reset,
		)

		if r.state == OK {
			continue
		}

		fmt.Printf("     %s%s%s\n", dim, r.req.Consequence, reset)

		if !r.req.Installable() && r.req.ManualHint != "" {
			fmt.Printf("     %s→ %s%s\n", yellow, r.req.ManualHint, reset)
		}

		if !r.req.Optional {
			blocking++
		}

		if r.req.Installable() {
			fixable++
		}
	}

	return blocking, fixable
}

// install runs the fix for one requirement. Output is streamed straight through
// rather than captured: these commands prompt for passwords and print progress,
// and swallowing that would leave someone staring at a frozen terminal.
func install(r Requirement) error {
	cmd := r.InstallCmd()
	if cmd == nil {
		return fmt.Errorf("no automatic install available")
	}

	fmt.Printf("\n%s→ %s%s\n%s  %s%s\n", bold, r.Name, reset, dim, strings.Join(cmd, " "), reset)

	if r.NeedsRoot {
		fmt.Printf("%s  (this needs your password)%s\n", dim, reset)
	}

	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin

	return c.Run()
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

	results := runChecks()

	allOK := true
	for _, r := range results {
		if r.state != OK && !r.req.Optional {
			allOK = false
		}
	}

	if allOK && *quiet && !*autoInstall {
		return
	}

	blocking, fixable := report(results)

	if blocking == 0 {
		fmt.Printf("\n%s✓ Ready.%s\n\n", green, reset)
	} else {
		fmt.Printf("\n%s%d requirement(s) must be met before Pnexus can run.%s\n", red, blocking, reset)
	}

	if !*autoInstall {
		if fixable > 0 {
			fmt.Printf("%sRun 'pnexus-doctor --install' to fix %d of them automatically.%s\n\n", dim, fixable, reset)
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

	failed := 0

	for _, r := range results {
		if r.state == OK || !r.req.Installable() {
			continue
		}

		if !*assumeYes && !confirm(fmt.Sprintf("Install %s? (%s)", r.req.Name, r.req.Why)) {
			fmt.Printf("%s  skipped%s\n", dim, reset)

			continue
		}

		if err := install(r.req); err != nil {
			fmt.Printf("%s  failed: %v%s\n", red, err, reset)
			failed++
		}
	}

	// Re-check rather than assuming the installs worked: an apt command can
	// exit zero having installed something that still doesn't satisfy the
	// check, and reporting success on that basis would be a lie.
	fmt.Printf("\n%sRe-checking…%s\n", dim, reset)

	if blockingAfter, _ := report(runChecks()); blockingAfter > 0 {
		fmt.Printf("\n%s%d still unmet.%s\n\n", red, blockingAfter, reset)
		os.Exit(1)
	}

	fmt.Printf("\n%s✓ Ready.%s\n\n", green, reset)

	if failed > 0 {
		os.Exit(1)
	}
}
