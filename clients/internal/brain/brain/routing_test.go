package brain

import (
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/ask"
	"pn-scripts-assistant/internal/brain/llm"
)

// A reply to a question with answers is the answer it picked, by number or
// word, in either language; anything else is kept as written.
func TestAReplyToAQuestionIsTheAnswerItPicked(t *testing.T) {
	b := testBrain(t)

	conv, err := b.DB.NewConversation("t")
	if err != nil {
		t.Fatal(err)
	}

	q := ask.Question{Text: "Къде да живее мозъкът?", Options: []string{"На флашка", "На този компютър"}, Recommended: 1}
	b.DB.AddMessage(conv, llm.RoleUser, "", "", "Настрой ме")
	b.DB.AddMessage(conv, llm.RoleAssistant, "", "", ask.Show(q, "bg"))

	for reply, want := range map[string]string{
		"1":               "На този компютър",
		"второто":         "На флашка",
		"флашка":          "На флашка",
		"на моя NAS диск": "на моя NAS диск",
	} {
		if got := b.answerToQuestion(conv, reply); got != want {
			t.Errorf("%q became %q, want %q", reply, got, want)
		}
	}

	// Once answered, the next "2" is just "2".
	b.DB.AddMessage(conv, llm.RoleUser, "", "", "На флашка")

	if got := b.answerToQuestion(conv, "2"); got != "2" {
		t.Errorf("an old question still captured the reply: %q", got)
	}
}

// A new brain starts with the knowledge the program carries, in the folder
// that travels with it.
func TestANewBrainStartsWithTheSeedKnowledge(t *testing.T) {
	b := testBrain(t)

	if _, err := os.Stat(filepath.Join(b.Root, "knowledge", "guides", "getting-started.md")); err != nil {
		t.Fatalf("no starting knowledge: %v", err)
	}
}
