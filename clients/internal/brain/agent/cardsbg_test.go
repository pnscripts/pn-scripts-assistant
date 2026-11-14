package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// installingBoth says its card in both languages.
type installingBoth struct{ installing }

func (i installingBoth) Summarize(a json.RawMessage) string { return i.SummarizeIn(a, "en") }

func (installingBoth) SummarizeIn(_ json.RawMessage, lang string) string {
	return tools.Say(lang, "Install Godot 4.7.2", "Инсталиране на Godot 4.7.2")
}

/*
 * The question is kept in both languages: English for the model, the deeds
 * and the rules, Bulgarian for a page in Bulgarian, with every reason it is
 * asked translated around the same thing asked about. Asked in Bulgarian,
 * the note in the conversation is Bulgarian too.
 */
func TestTheQuestionIsKeptInBothLanguages(t *testing.T) {
	for _, c := range []struct {
		said, note, card string
	}{
		{"Инсталирай Godot, моля", "Това изисква вашето одобрение, преди да го направя:",
			"Винаги се пита (инсталиране на софтуер): Инсталиране на Godot 4.7.2"},
		{"Install Godot, please", "This needs your approval before I can do it:",
			"Always asked (installing software): Install Godot 4.7.2"},
	} {
		ran := 0

		loop, db := newLoop(t, installingBoth{installing{counted{name: "install_requirement", ran: &ran}}})
		loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

		conv, _ := db.NewConversation("t")

		res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"),
			[]llm.Message{{Role: llm.RoleUser, Content: c.said}}, Brief{WithTools: true})
		if err != nil {
			t.Fatal(err)
		}

		if ran != 0 || len(res.Pending) != 1 {
			t.Fatalf("ran %d, pending %+v", ran, res.Pending)
		}

		p := res.Pending[0]

		if p.Summary != "Always asked (installing software): Install Godot 4.7.2" ||
			p.SummaryBG != "Винаги се пита (инсталиране на софтуер): Инсталиране на Godot 4.7.2" {
			t.Errorf("the cards: %q / %q", p.Summary, p.SummaryBG)
		}

		waiting, err := db.PendingInvocations()
		if err != nil || len(waiting) != 1 || waiting[0].Summary != p.Summary || waiting[0].SummaryBG != p.SummaryBG {
			t.Errorf("kept as %+v (%v)", waiting, err)
		}

		if !strings.Contains(res.Reply, c.note) || !strings.Contains(res.Reply, "  • "+c.card) {
			t.Errorf("said %q in the conversation, want %q and %q", res.Reply, c.note, c.card)
		}
	}
}
