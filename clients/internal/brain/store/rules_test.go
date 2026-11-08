package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/rules"
)

func proposal(t *testing.T, said string) rules.Rule {
	t.Helper()

	p, ok := rules.Notice(said)
	if !ok {
		t.Fatalf("%q was not noticed", said)
	}

	return p.Rule(7, time.Now())
}

func TestAProposalIsNotARuleUntilItIsKept(t *testing.T) {
	db := openDeeds(t)

	r, err := db.AddRule(proposal(t, "Never send emails to anna@example.org."))
	if err != nil {
		t.Fatal(err)
	}

	if r.Status != rules.Proposed || r.On {
		t.Fatalf("a proposal is live: %+v", r)
	}

	if live, _ := db.LiveRules(); len(live) != 0 {
		t.Fatalf("live before it was kept: %v", live)
	}

	// An edit cannot keep it.
	r.On = true
	if changed, _ := db.ChangeRule(r); changed.On || changed.Status != rules.Proposed {
		t.Fatalf("an edit kept it: %+v", changed)
	}

	kept, err := db.DecideRule(r.ID, true)
	if err != nil || kept.Status != rules.Kept || !kept.On {
		t.Fatalf("%+v %v", kept, err)
	}

	if _, err := db.DecideRule(r.ID, false); err == nil {
		t.Error("decided twice")
	}

	live, _ := db.LiveRules()
	if len(live) != 1 || live[0].Text != "Never send emails to anna@example.org." ||
		!reflect.DeepEqual(live[0].Words, []string{"anna@example.org"}) || live[0].ConversationID != 7 ||
		live[0].Source != "Never send emails to anna@example.org." {
		t.Fatalf("%+v", live)
	}
}

func TestRulesAreEditedSwitchedAndDeleted(t *testing.T) {
	db := openDeeds(t)

	r, _ := db.AddRule(proposal(t, "Never post on Sundays."))
	r, _ = db.DecideRule(r.ID, true)

	r.Text = "never post on Saturdays or Sundays"
	r.Days = []time.Weekday{time.Sunday, time.Saturday}
	r.Words = []string{"LinkedIn"}
	r.Actions = []string{rules.Post, "fly"}

	changed, err := db.ChangeRule(r)
	if err != nil {
		t.Fatal(err)
	}

	again, _ := db.Rule(r.ID)
	if again.Text != "Never post on Saturdays or Sundays." || !reflect.DeepEqual(again.Actions, []string{rules.Post}) ||
		!reflect.DeepEqual(again.Days, []time.Weekday{time.Saturday, time.Sunday}) ||
		!reflect.DeepEqual(again.Words, []string{"LinkedIn"}) || again.Status != rules.Kept || !again.On {
		t.Fatalf("%+v (returned %+v)", again, changed)
	}

	if off, _ := db.SwitchRule(r.ID, false); off.On {
		t.Error("still on")
	}

	if live, _ := db.LiveRules(); len(live) != 0 {
		t.Errorf("a switched-off rule is live: %v", live)
	}

	if err := db.RemoveRule(r.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Rule(r.ID); err != ErrNoRule {
		t.Errorf("%v", err)
	}

	if err := db.RemoveRule(r.ID); err != ErrNoRule {
		t.Errorf("deleted twice: %v", err)
	}
}

func TestADeclinedSentenceIsNotProposedAgain(t *testing.T) {
	db := openDeeds(t)

	r, _ := db.AddRule(proposal(t, "Never push to main."))
	_, _ = db.DecideRule(r.ID, false)

	if known, _ := db.KnownRule("never push to main"); !known {
		t.Error("a declined rule was forgotten")
	}

	if list, _ := db.Rules(false); len(list) != 0 {
		t.Errorf("a declined rule is listed: %v", list)
	}

	if _, err := db.SwitchRule(r.ID, true); err == nil {
		t.Error("a declined rule was switched on")
	}
}

func TestProposalsComeFirst(t *testing.T) {
	db := openDeeds(t)

	a, _ := db.AddRule(proposal(t, "Never push to main."))
	_, _ = db.DecideRule(a.ID, true)
	b, _ := db.AddRule(proposal(t, "Always ask me before you send an email."))

	list, _ := db.Rules(false)
	if len(list) != 2 || list[0].ID != b.ID {
		t.Errorf("%v", list)
	}
}

func TestRulesAreSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	r := proposal(t, "Never send emails to Petrova about the Rosewood contract.")
	r.Words = []string{"Petrova"}

	if _, err := d.AddRule(r); err != nil {
		t.Fatal(err)
	}

	if list, _ := d.Rules(false); len(list) != 1 || !strings.Contains(list[0].Text, "Rosewood") {
		t.Fatalf("%v", list)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	for _, word := range []string{"Rosewood", "Petrova"} {
		if strings.Contains(string(raw)+string(wal), word) {
			t.Errorf("%q is readable on disk", word)
		}
	}
}
