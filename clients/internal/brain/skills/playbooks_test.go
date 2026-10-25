package skills

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

// What its owner's work was, written down (ADR 0007). Each is checked by name
// so that losing one is a failure, not a quieter list.
//
// Only what every user needs (owner 23.10: "само тези от които има нужда"):
// asking, planning a goal, mail, documents and the scam guard. The twelve
// procedures of its owner's own business left the binary — they live in his
// own skills and knowledge, which the program imports.
var wantPlaybooks = []string{
	"ask_one_question", "handle_email", "plan_a_goal", "scam_guard", "work_with_documents",
}

// Personal procedures are not shipped, and a test says so rather than a list
// that is quietly one shorter.
var personalPlaybooks = []string{
	"email_and_complaints", "planning_loans_strategy",
	"brand_naming", "build_html5_games", "build_products", "github_cost_control",
	"machine_throttling", "publish_game_jolt", "publish_itch_io", "sales_hold",
	"ship_pnscripts_com", "update_ai_brain", "update_social_profiles", "wordpress_org_plugin_review",
}

func TestPersonalProceduresAreNotShipped(t *testing.T) {
	found, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != len(wantPlaybooks) {
		t.Errorf("%d playbooks shipped, want %d", len(found), len(wantPlaybooks))
	}

	for _, s := range found {
		for _, personal := range personalPlaybooks {
			if s.Name == personal {
				t.Errorf("%s is its owner's own and must not ship in the binary", personal)
			}
		}
	}
}

func TestEveryPlaybookShipsVersionedAndReadable(t *testing.T) {
	found, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]Skill{}
	for _, s := range found {
		byName[s.Name] = s
	}

	for _, name := range wantPlaybooks {
		s, ok := byName[name]
		if !ok {
			t.Errorf("the %s playbook is not shipped", name)

			continue
		}

		if s.Version < 1 || !s.Builtin || strings.TrimSpace(s.When) == "" || len(s.How) < 1000 {
			t.Errorf("%s: version %d, builtin %v, when %q, %d characters", name, s.Version, s.Builtin, s.When, len(s.How))
		}

		for _, section := range []string{"## Before you start", "## Verify", "## Never"} {
			if !strings.Contains(s.How, section) {
				t.Errorf("%s has no %q section", name, section)
			}
		}
	}
}

/*
 * Nothing private ships in the binary: no address on somebody's own machine,
 * no key-shaped string, no IBAN, no telephone number.
 */
func TestNoPlaybookCarriesSecretsOrPersonalData(t *testing.T) {
	found, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}

	bad := map[string]*regexp.Regexp{
		"a home folder":    regexp.MustCompile(`/home/[a-z]+/|/media/[a-z]+/`),
		"an IBAN":          regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z]{4}\d{6,}`),
		"a telephone":      regexp.MustCompile(`\+\d{3}[ \d]{7,}`),
		"a key":            regexp.MustCompile(`(sk-[A-Za-z0-9]{16,}|ghp_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16})`),
		"a password value": regexp.MustCompile(`(?i)password\s*[:=]\s*\S+`),
	}

	for _, s := range found {
		for what, re := range bad {
			if m := re.FindString(s.How + s.When); m != "" {
				t.Errorf("%s contains %s: %q", s.Name, what, m)
			}
		}
	}
}

// A playbook is offered by its own words and says which version it is.
func TestAPlaybookIsOfferedByItsWords(t *testing.T) {
	found, _ := Builtin()

	for _, s := range found {
		if s.Name != "handle_email" {
			continue
		}

		tool := AsTool(s)

		if !strings.Contains(tool.Description(), "version 2") {
			t.Errorf("the description does not say the version: %q", tool.Description())
		}

		cues := strings.Join(tool.(tools.Cued).Cues(), " ")
		if !strings.Contains(cues, "inbox") {
			t.Errorf("inbox is not a cue: %s", cues)
		}

		out, err := tool.Execute(context.Background(), nil)
		if err != nil || !strings.Contains(out, "send it with send_draft only after the owner says yes") {
			t.Errorf("reading it did not return the procedure: %v %q", err, out[:min(200, len(out))])
		}

		if tool.Risk() != tools.Safe {
			t.Error("reading a playbook changes something")
		}

		return
	}

	t.Fatal("handle_email is not shipped")
}

// Its owner's own skill of the same name wins, so a playbook can be corrected
// without waiting for a release.
func TestTheOwnersOwnVersionWins(t *testing.T) {
	root := t.TempDir()

	if err := Save(root, Skill{Name: "handle_email", When: "selling", How: "Mine."}); err != nil {
		t.Fatal(err)
	}

	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}

	count := 0

	for _, s := range all {
		if s.Name == "handle_email" {
			count++

			if s.Builtin || s.How != "Mine." {
				t.Errorf("the shipped one won: %+v", s)
			}
		}
	}

	if count != 1 || len(all) != len(wantPlaybooks) {
		t.Errorf("%d handle_email, %d skills in all", count, len(all))
	}
}
