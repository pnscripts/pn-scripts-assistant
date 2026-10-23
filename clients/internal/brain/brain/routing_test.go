package brain

import (
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/ask"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/route"
	"pn-scripts-assistant/internal/brain/store"
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
		// As speech recognition writes a spoken answer: capital, full stop.
		"Второто.":       "На флашка",
		"Две.":           "На флашка",
		"Първото, моля.": "На този компютър",
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

// What testing a model here showed reaches the router.
func TestMeasuredSpeedReachesTheRouter(t *testing.T) {
	b := testBrain(t)

	if err := b.DB.RecordModelTest(store.ModelTest{Name: "llama3.2:3b", Seconds: 3.5, ToolCall: "proper"}); err != nil {
		t.Fatal(err)
	}

	got := b.measured([]route.Candidate{{Name: "llama3.2:3b"}, {Name: "never-tested:7b"}})

	if got[0].Seconds != 3.5 || got[0].ToolCall != "proper" {
		t.Fatalf("the measurement did not reach the candidate: %+v", got[0])
	}

	if got[1].Seconds != 0 || got[1].ToolCall != "" {
		t.Fatalf("an untested model got a measurement: %+v", got[1])
	}
}
