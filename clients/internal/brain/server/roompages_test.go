package server

import (
	"path/filepath"
	"regexp"
	"testing"

	"pn-scripts-assistant/internal/brain/skills"
	"pn-scripts-assistant/internal/brain/tasks"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * The names the room, Teaching, Tasks and Skills pages take from the program
 * are said in both languages.
 *
 * The built-in specialists are named and described in team.go in English,
 * which is what the planner reads; the page says them from i18n.js, so a
 * specialist added or renamed there without its Bulgarian shows here. The
 * same for the built-in playbooks, which have no Bulgarian file of their own.
 */
func TestTheBuiltInSpecialistsAndPlaybooksHaveBulgarianNames(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	for _, agent := range team.BuiltIn() {
		for key, english := range map[string]string{
			"task.agent." + agent.Name:    agent.Title,
			"task.agentFor." + agent.Name: agent.For,
		} {
			en, bg := strings2(t, js, key)
			if en != english {
				t.Errorf("%s: team.go says %q and i18n.js %q", key, english, en)
			}

			if !cyrillic.MatchString(bg) {
				t.Errorf("%s has no Bulgarian: %q", key, bg)
			}
		}
	}

	playbooks, err := skills.Builtin()
	if err != nil {
		t.Fatal(err)
	}

	if len(playbooks) == 0 {
		t.Fatal("no built-in playbooks: has the list moved?")
	}

	for _, p := range playbooks {
		if _, bg := strings2(t, js, "teach.playbook."+p.Name); !cyrillic.MatchString(bg) {
			t.Errorf("the playbook %s has no Bulgarian name: %q", p.Name, bg)
		}
	}
}

/*
 * The reasons a task gives for waiting or stopping, which the program writes
 * in fixed English words, are each said in Bulgarian: the server sends their
 * key, and i18n.js has it with the same English.
 */
func TestTheFixedReasonsATaskGivesAreSaidInBulgarian(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	fixed := []string{
		tasks.ReasonAsked, tasks.ReasonDecision, tasks.ReasonStopped, tasks.ReasonNotRunning, tasks.ReasonSlow,
		tasks.ReasonMostSteps, tasks.ReasonNoBuild, tasks.ReasonHalfHour, tasks.ReasonThinking,
	}

	for _, because := range fixed {
		key, _ := tasks.ReasonKey(because)
		if key == "" {
			t.Errorf("%q has no key", because)

			continue
		}

		if en, _ := strings2(t, js, key); en != because {
			t.Errorf("%s: the program says %q and i18n.js %q", key, because, en)
		}
	}

	for _, key := range tasks.ReasonKeys() {
		if _, bg := strings2(t, js, key); !cyrillic.MatchString(bg) {
			t.Errorf("%s has no Bulgarian: %q", key, bg)
		}
	}

	// The two that carry something else: an error, and who it waits on.
	if key, args := tasks.ReasonKey("it stopped on an error: " + tasks.ReasonSlow); key != "task.why.error" ||
		args["why"] != tasks.ReasonSlow || args["why_key"] != "task.why.slow" {
		t.Errorf("an error that is itself a reason: %s %v", key, args)
	}

	if key, args := tasks.ReasonKey("waiting on the game developer"); key != "task.why.waitingOn" || args["who"] != "game developer" {
		t.Errorf("waiting on a specialist: %s %v", key, args)
	}

	if key, _ := tasks.ReasonKey("dial tcp: connection reset"); key != "" {
		t.Errorf("an error from elsewhere was given a key: %s", key)
	}
}
