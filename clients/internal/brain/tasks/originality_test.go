package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

func checkers(p Plan) []string {
	var out []string

	for _, s := range p.Steps {
		if strings.HasPrefix(s.Instruction, OriginalityMarker) {
			out = append(out, s.Assignee)
		}
	}

	return out
}

// Each result is checked by the specialists of the fields it touches, side
// by side, and only those — with the reasons written on the first check.
func TestEachResultIsCheckedByTheSpecialistsItNeedsOnly(t *testing.T) {
	base := Plan{Steps: []store.TaskStep{{Instruction: "do it"}, {Instruction: "finish it"}}}

	cases := map[string][]string{
		"Make me a great space strategy game":            {"visual_reviewer", "game_design_reviewer", "copy_reviewer", "business_strategist"},
		"Направи ми лого за пекарна":                     {"ip_specialist", "visual_reviewer"},
		"compose a jingle for the radio":                 {"sound_reviewer"},
		"Make a business strategy for a bakery in Sofia": {"copy_reviewer", "business_strategist"},
		"Направи бюджет и план за ДДС на фирмата":        {"copy_reviewer", "finance_analyst", "accountant"},
		"write a draft logo and slogan for myself":       {"ip_specialist"},
		"sort my downloads folder":                       nil,
	}

	for request, want := range cases {
		got := withOriginality(request, base)

		if strings.Join(checkers(got), ",") != strings.Join(want, ",") {
			t.Errorf("%q checked by %v, want %v", request, checkers(got), want)

			continue
		}

		if len(want) == 0 {
			continue
		}

		first := got.Steps[2]
		if !strings.Contains(first.Instruction, "not needed") && len(want) < len(aspects) {
			t.Errorf("%q: the reasons for the checks not used are not recorded: %s", request, first.Instruction)
		}

		for i, s := range got.Steps[2:] {
			if s.Kind != store.StepCheck || s.Together != (i > 0) {
				t.Errorf("%q: check %d is not a parallel check step: %+v", request, i, s)
			}
		}

		if again := withOriginality(request, got); len(again.Steps) != len(got.Steps) {
			t.Errorf("%q: checks added twice", request)
		}
	}
}

// The specialists' findings are one section of the final report, under their
// own heading, not more bullets among the answers.
func TestTheChecksHaveTheirOwnSectionInTheReport(t *testing.T) {
	plan := withOriginality("Направи ми лого за пекарна", Plan{Steps: []store.TaskStep{{Instruction: "draw the logo"}}})

	steps := plan.Steps
	steps[0].State, steps[0].Answer, steps[0].Verdict = store.StepDone, "logo.svg drawn", store.Verified
	steps[1].State, steps[1].Answer = store.StepDone, "no registered mark named Zorna in TMview"
	steps[2].State = store.StepRunning

	report := (&Conductor{}).report(&store.Task{Name: "logo"}, steps, store.TaskDone, "done")

	at := strings.Index(report, OriginalitySection)
	if at < 0 {
		t.Fatalf("no originality section:\n%s", report)
	}

	answers, section := report[:at], report[at:]

	if !strings.Contains(answers, "logo.svg drawn") || strings.Contains(answers, "TMview") {
		t.Errorf("the checks are mixed with the answers:\n%s", report)
	}

	for _, want := range []string{"• name and trademarks: no registered mark named Zorna", "• look: not done"} {
		if !strings.Contains(section, want) {
			t.Errorf("section lacks %q:\n%s", want, section)
		}
	}

	if plain := (&Conductor{}).report(&store.Task{Name: "sort"}, steps[:1], store.TaskDone, "done"); strings.Contains(plain, OriginalitySection) {
		t.Errorf("a result without checks got the section:\n%s", plain)
	}
}

// A check that finds something in the job's own work is followed by the fix,
// by whoever made it, and one check again — without anybody being asked.
func TestWhatAChecksFindsInItsOwnWorkIsFixedAndCheckedAgain(t *testing.T) {
	c, db, _ := newConductor(t, nil)

	id, err := db.NewTask(store.Task{Name: "logo", State: store.TaskWorking})
	if err != nil {
		t.Fatal(err)
	}

	plan := withOriginality("Направи ми лого за пекарна", Plan{Steps: []store.TaskStep{{Instruction: "draw the logo", Assignee: "designer"}}})
	if err := db.AddSteps(id, plan.Steps); err != nil {
		t.Fatal(err)
	}

	steps, _ := db.Steps(id)
	steps[0].State, steps[0].Answer = store.StepDone, "logo.svg"
	_ = db.FinishStep(steps[0])

	check := steps[1]
	check.State, check.Answer = store.StepDone, "OK: no registered mark of that name"

	task, _ := db.Task(id)
	if err := c.afterCheck(task, &check); err != nil {
		t.Fatal(err)
	}

	if after, _ := db.Steps(id); len(after) != len(steps) {
		t.Fatalf("a passed check added steps: %d", len(after))
	}

	look := steps[2]
	look.State, look.Answer = store.StepDone, "**CHANGE:** the wheat sheaf copies the Kamps logo; draw a loaf instead"

	if err := c.afterCheck(task, &look); err != nil {
		t.Fatal(err)
	}

	after, _ := db.Steps(id)
	if len(after) != len(steps)+2 {
		t.Fatalf("want a fix and a check again, got %d steps", len(after))
	}

	fix, again := after[len(after)-2], after[len(after)-1]
	if fix.Assignee != "designer" || !fix.Changes || !strings.Contains(fix.Instruction, "draw a loaf") {
		t.Errorf("fix: %+v", fix)
	}

	if again.Assignee != "visual_reviewer" || aspectOf(again.Instruction) != "look" || !strings.Contains(again.Instruction, "(look, again)") {
		t.Errorf("check again: %+v", again)
	}

	// The second check's finding is reported, not fixed round and round.
	again.State, again.Answer = store.StepDone, "CHANGE: still close"
	if err := c.afterCheck(task, &again); err != nil {
		t.Fatal(err)
	}

	if final, _ := db.Steps(id); len(final) != len(after) {
		t.Error("a check again started another round")
	}
}

// An audit is of the user's works: the specialists come only for the aspects
// with findings, and nothing is changed.
func TestAnAuditBringsOnlyTheSpecialistsItsFindingsNeed(t *testing.T) {
	for request, folder := range map[string]string{
		"Audit the originality of everything in ~/Games/catalogue": "~/Games/catalogue",
		"Провери оригиналността на игрите в /home/ana/игри":        "/home/ana/игри",
	} {
		p, ok := auditPlan(request)
		if !ok || len(p.Steps) != 1 || !strings.Contains(p.Steps[0].Instruction, folder) {
			t.Errorf("%q: %+v", request, p)
		}
	}

	for _, not := range []string{"Направи ми игра и провери оригиналността", "sort my downloads", "check the weather"} {
		if _, ok := auditPlan(not); ok {
			t.Errorf("%q became an audit", not)
		}
	}

	old := tools.Root
	tools.Root = t.TempDir()
	t.Cleanup(func() { tools.Root = old })

	dir := filepath.Join(tools.Root, "reports", "originality")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "originality-audit-1.csv"), []byte(
		"item,aspect,finding,risk,action,fixed\n"+
			"Tetriz,name and trademarks,Tetris,high,rename,\n"+
			"Tetriz,sound,Korobeiniki,medium,compose,\n"+
			"Moonfold,look,no references,low,search,\n"), 0o644)

	c, db, _ := newConductor(t, nil)
	p, _ := auditPlan("audit originality of ~/x")
	id, _ := db.NewTask(store.Task{Name: "audit", State: store.TaskWorking})
	_ = db.AddSteps(id, p.Steps)
	steps, _ := db.Steps(id)
	steps[0].State, steps[0].Answer = store.StepDone, "CHANGE: saved"

	task, _ := db.Task(id)
	if err := c.afterCheck(task, &steps[0]); err != nil {
		t.Fatal(err)
	}

	after, _ := db.Steps(id)

	var who []string
	for _, s := range after[1:] {
		who = append(who, s.Assignee)

		if s.Changes || !strings.Contains(s.Instruction, "change nothing") {
			t.Errorf("a review may change the user's work: %+v", s)
		}
	}

	if strings.Join(who, ",") != "ip_specialist,sound_reviewer" {
		t.Errorf("reviewed by %v, want the name and sound specialists only", who)
	}

	// A review that asks for a change in the user's work is reported only.
	review := after[1]
	review.State, review.Answer = store.StepDone, "CHANGE: rename Tetriz"

	if err := c.afterCheck(task, &review); err != nil {
		t.Fatal(err)
	}

	if final, _ := db.Steps(id); len(final) != len(after) {
		t.Error("an audit finding was fixed in the user's own work")
	}
}
