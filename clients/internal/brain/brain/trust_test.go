package brain

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

// citing is a local model that answers with whatever it is told to, and
// remembers what it was asked. It embeds everything the same way, so every
// remembered fact matches every question.
type citing struct {
	mu    sync.Mutex
	reply string
	seen  []llm.Request
}

func (*citing) Name() string                   { return "ollama" }
func (*citing) Available(context.Context) bool { return true }
func (*citing) EmbedModel() string             { return "same" }

func (*citing) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0}, nil
}

func (c *citing) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.seen = append(c.seen, req)

	return llm.Response{Content: c.reply}, nil
}

/*
 * The whole trust layer for a typed answer: the model reads what it
 * remembers with numbers on it, an invented number is taken out of the
 * answer, what it cited is marked, and the list is kept with the message so a
 * conversation opened later still says where its answers came from.
 */
func TestATypedAnswerSaysWhereItCameFrom(t *testing.T) {
	b := quietBrain(t)

	model := &citing{reply: "The meeting is on Tuesday [1]. It is in Sofia [7]."}

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", model)
	b.Agent.Model = nil

	if _, err := b.DB.AddFact("document", "The team meeting is on Tuesday at ten.", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}

	if _, err := b.DB.SQL().Exec(`UPDATE knowledge_facts SET source = 'document:/home/you/plan.md'`); err != nil {
		t.Skip("facts carry no source column here: " + err.Error())
	}

	reply, err := b.Chat(context.Background(), ChatRequest{Message: "When is the team meeting?", Provider: "ollama"})
	if err != nil {
		t.Fatal(err)
	}

	if reply.Reply != "The meeting is on Tuesday [1]. It is in Sofia." {
		t.Errorf("the answer reads %q", reply.Reply)
	}

	if len(reply.Sources) != 1 || !reply.Sources[0].Cited || reply.Sources[0].Title != "plan.md" {
		t.Fatalf("sources %+v", reply.Sources)
	}

	// The model was shown the fact with its number and the place's name.
	shown := false

	for _, req := range model.seen {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "[1] The team meeting is on Tuesday at ten. (from: plan.md)") {
				shown = true
			}
		}
	}

	if !shown {
		t.Error("the model never saw the numbered fact")
	}

	// And the conversation keeps them.
	history, err := b.DB.History(reply.ConversationID)
	if err != nil {
		t.Fatal(err)
	}

	last := history[len(history)-1]

	var kept []map[string]any
	if err := json.Unmarshal(last.Sources, &kept); err != nil || len(kept) != 1 {
		t.Fatalf("the stored answer has sources %s (%v)", last.Sources, err)
	}

	if last.Content != reply.Reply {
		t.Errorf("the stored answer %q is not the checked one %q", last.Content, reply.Reply)
	}
}

// Switched off, nothing asks the model to cite — but what it drew on is still
// listed, because that is not the model's to hide.
func TestWithCitingOffTheSourcesAreStillListed(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.CiteSources = false

	model := &citing{reply: "On Tuesday."}

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", model)
	b.Agent.Model = nil

	b.DB.AddFact("document", "The team meeting is on Tuesday at ten.", []float32{1, 0, 0})

	reply, err := b.Chat(context.Background(), ChatRequest{Message: "When is the team meeting?", Provider: "ollama"})
	if err != nil {
		t.Fatal(err)
	}

	for _, req := range model.seen {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "square brackets") {
				t.Fatalf("the model was asked to cite with citing off:\n%s", m.Content)
			}
		}
	}

	if len(reply.Sources) != 1 || reply.Sources[0].Cited {
		t.Errorf("sources %+v", reply.Sources)
	}
}

// Every call is in the record with the request that led to it.
func TestADeedIsRecordedWithItsReason(t *testing.T) {
	b := quietBrain(t)

	b.recordDeed(deedFor("read_file", `{"path":"/etc/hostname"}`, "what is this machine called?"))
	b.recordDeed(deedFor("fetch_url", `{"url":"https://www.example.org/x"}`, "read that page"))

	deeds, err := b.DB.Deeds(store.DeedFilter{})
	if err != nil {
		t.Fatal(err)
	}

	if len(deeds) != 2 {
		t.Fatalf("%d deeds", len(deeds))
	}

	if deeds[0].Tool != "fetch_url" || deeds[0].Leaves != "example.org" || deeds[0].Request != "read that page" {
		t.Errorf("the page read reads %+v", deeds[0])
	}

	if deeds[1].Leaves != "" || deeds[1].Changes {
		t.Errorf("a file read says it left or changed something: %+v", deeds[1])
	}
}

func deedFor(tool, args, request string) agent.Deed {
	return agent.Deed{
		Step:    agent.Step{Tool: tool, Summary: tool},
		Args:    json.RawMessage(args),
		Request: request,
		Outcome: agent.DeedDone,
	}
}

/*
 * The record says what was done in Bulgarian too: the card a call carried
 * through the loop, and the card a question was stored with when it is
 * decided after the turn that asked it.
 */
func TestADeedIsRecordedInBothLanguages(t *testing.T) {
	b := quietBrain(t)

	d := deedFor("read_file", `{"path":"/etc/hostname"}`, "как се казва тази машина?")
	d.Step.Summary, d.Step.SummaryBG = "Read /etc/hostname", "Прочитане на /etc/hostname"

	b.recordDeed(d)

	asked := store.Invocation{Tool: "write_file", Arguments: `{"path":"notes.txt"}`,
		Summary: "Create notes.txt", SummaryBG: "Създаване на notes.txt"}

	b.recordDenied(asked)
	b.recordInvocation(asked, agent.DeedDone, "", true)

	deeds, err := b.DB.Deeds(store.DeedFilter{})
	if err != nil || len(deeds) != 3 {
		t.Fatalf("%+v (%v)", deeds, err)
	}

	if deeds[2].Summary != "Read /etc/hostname" || deeds[2].SummaryBG != "Прочитане на /etc/hostname" {
		t.Errorf("a call: %+v", deeds[2])
	}

	for _, decided := range deeds[:2] {
		if decided.Summary != "Create notes.txt" || decided.SummaryBG != "Създаване на notes.txt" {
			t.Errorf("a decided question: %+v", decided)
		}
	}

	if deeds[1].Outcome != agent.DeedRefused || deeds[0].Outcome != agent.DeedDone {
		t.Errorf("outcomes %s, %s", deeds[1].Outcome, deeds[0].Outcome)
	}
}
