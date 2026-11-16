package brain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * From the command the model writes to the refusal: a "never" on a
 * program's card reaches the program wherever the line starts it, after
 * the first command, inside a nested shell, behind sudo, named in a
 * one-line script (leftover of the 1.1.0 security review). A line that
 * starts nothing refused is left to the usual questions.
 */
func TestANeverReachesEveryCommandOfTheLine(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Freedom = string(permits.Everything)

	// A program that is really here, so a name finds it: the test's own
	// shell.
	const name = "sh"

	save, _ := programs.Reconcile(programs.App, nil,
		[]programs.Program{{Kind: programs.App, ID: "zz-line-demo.desktop", Name: "Line Demo", Exec: "true"}}, time.Now())

	if err := b.DB.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	refused := func(tool tools.Tool, argv ...string) string {
		raw, _ := json.Marshal(map[string]any{"argv": argv, "why": "test"})

		return b.refusedByItsCard(context.Background(), tool, raw)
	}

	lines := [][]string{
		{name, "-c", "echo hi; true"},
		{name, "-c", "echo hi && echo there | true"},
		{name, "-c", "echo $(true)"},
		{name, "-c", "echo hi; " + name + " -c 'echo there; true'"},
		{name, "-c", "echo hi; env A=1 nohup true"},
		{"python3", "-c", "import os; os.system('true')"},
	}

	// Nothing decided: nothing is refused.
	for _, argv := range lines {
		if why := refused(tools.RunCommand{}, argv...); why != "" {
			t.Errorf("%s: refused with nothing decided: %s", strings.Join(argv, " "), why)
		}
	}

	if _, err := b.SetProgramPermit(programs.App, "zz-line-demo.desktop", programs.VerbChange, programs.Never); err != nil {
		t.Fatal(err)
	}

	for _, argv := range lines {
		why := refused(tools.RunCommand{}, argv...)

		if !strings.Contains(why, "Line Demo") || !strings.Contains(why, "never") {
			t.Errorf("%s: the never did not reach it: %q", strings.Join(argv, " "), why)
		}

		if why := refused(tools.RunAsAdmin{}, argv...); !strings.Contains(why, "Line Demo") {
			t.Errorf("as administrator, %s: the never did not reach it: %q", strings.Join(argv, " "), why)
		}
	}

	// A line that does not start it is not refused by it.
	for _, argv := range [][]string{
		{name, "-c", "echo hi; echo 'true'"},
		{name, "-c", "echo hi > /tmp/true"},
		{"echo", "true"},
	} {
		if why := refused(tools.RunCommand{}, argv...); why != "" {
			t.Errorf("%s: refused: %s", strings.Join(argv, " "), why)
		}
	}

	// The loop's own question gives the same answer.
	raw, _ := json.Marshal(map[string]any{"argv": lines[0]})

	r := b.AlsoRefused(context.Background(), tools.AlsoRunsOf(context.Background(), tools.RunCommand{}, raw))
	if r.Answer != programs.Never || r.Program == nil || r.Program.Name != "Line Demo" {
		t.Errorf("the loop is told %+v", r)
	}

	if r := b.AlsoRefused(context.Background(), nil); r.Answer == programs.Never {
		t.Errorf("no programs: %+v", r)
	}
}
