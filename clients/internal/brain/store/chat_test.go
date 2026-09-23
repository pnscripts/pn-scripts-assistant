package store

import (
	"strings"
	"testing"
)

/*
 * A conversation can be named and removed.
 *
 * Titles are taken from the opening message, which is a fair guess and often a
 * poor name — one that began "can you hear me" is not about that, and twenty
 * such rows cannot be told apart.
 */
func TestAConversationCanBeRenamedAndDeleted(t *testing.T) {
	db := open(t)

	id, err := db.NewConversation("can you hear me")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.AddMessage(id, "user", "", "", "can you hear me"); err != nil {
		t.Fatal(err)
	}

	if err := db.RenameConversation(id, "Finding my projects"); err != nil {
		t.Fatalf("renaming: %v", err)
	}

	recent, err := db.RecentConversations(10)
	if err != nil {
		t.Fatal(err)
	}

	var found bool

	for _, c := range recent {
		if c.ID == id {
			found = true

			if c.Title != "Finding my projects" {
				t.Errorf("the new name is not shown: %q", c.Title)
			}
		}
	}

	if !found {
		t.Fatal("the conversation vanished from the list after being renamed")
	}

	// An empty name is not a name, and would leave a row nothing identifies.
	if err := db.RenameConversation(id, "   "); err == nil {
		t.Error("a blank name was accepted")
	}

	if err := db.DeleteConversation(id); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if exists, _ := db.ConversationExists(id); exists {
		t.Error("it is still there after being deleted")
	}

	// And deleting one that is gone says so rather than reporting success.
	if err := db.DeleteConversation(id); err == nil {
		t.Error("deleting a conversation that does not exist was reported as working")
	}
}

/*
 * Forgetting the last exchange takes the pair, never half of it.
 *
 * Removing a question and leaving its answer makes the record say the brain
 * volunteered something nobody asked for. Removing an answer and leaving the
 * question makes it say the brain was asked and ignored it. Both are worse
 * than whatever was being corrected.
 */
func TestForgettingTheLastExchangeTakesThePair(t *testing.T) {
	db := open(t)

	conv, err := db.NewConversation("t")
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range []struct{ role, content string }{
		{"system", "you are a brain"},
		{"user", "what is the first thing"},
		{"assistant", "the first answer"},
		{"user", "what is the second thing"},
		{"tool", "[looked something up] ..."},
		{"assistant", "the second answer"},
	} {
		if _, err := db.AddMessage(conv, m.role, "", "", m.content); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := db.ForgetLastExchange(conv)
	if err != nil {
		t.Fatal(err)
	}

	// The question, the tool output that served it, and the answer.
	if removed != 3 {
		t.Errorf("removed %d messages, want 3", removed)
	}

	left, err := db.History(conv)
	if err != nil {
		t.Fatal(err)
	}

	var said []string

	for _, m := range left {
		said = append(said, m.Role+":"+m.Content)
	}

	want := []string{
		"system:you are a brain",
		"user:what is the first thing",
		"assistant:the first answer",
	}

	if len(said) != len(want) {
		t.Fatalf("what is left is %v", said)
	}

	for i := range want {
		if said[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, said[i], want[i])
		}
	}

	// And again, taking the exchange before it.
	if _, err := db.ForgetLastExchange(conv); err != nil {
		t.Fatal(err)
	}

	after, _ := db.History(conv)

	if len(after) != 1 || after[0].Role != "system" {
		t.Errorf("the second forget left %+v", after)
	}

	// Nothing left to forget is not an error; it is an answer.
	if n, err := db.ForgetLastExchange(conv); err != nil || n != 0 {
		t.Errorf("forgetting an empty conversation returned %d, %v", n, err)
	}
}

/*
 * A secret printed by a tool does not stay in the conversation.
 *
 * The conversation is the dangerous place for one: it is read back into the
 * next turn and may be given to a service somewhere else, so a token that a
 * command printed once would be kept and sent onward forever after.
 */
func TestASecretInAMessageIsNotKept(t *testing.T) {
	db := open(t)

	id, err := db.NewConversation("checking")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.AddMessage(id, "tool", "", "",
		"[ran env]\nANTHROPIC_API_KEY=sk-ant-api03-QQQQPPPPOOOONNNNMMMM\nHOME=/home/somebody"); err != nil {
		t.Fatal(err)
	}

	history, err := db.History(id)
	if err != nil {
		t.Fatal(err)
	}

	if len(history) != 1 {
		t.Fatalf("expected one message, got %d", len(history))
	}

	if strings.Contains(history[0].Content, "sk-ant-api03-QQQQPPPPOOOONNNNMMMM") {
		t.Errorf("the key is in the conversation: %q", history[0].Content)
	}

	// Still a record of what happened, not an empty line.
	for _, want := range []string{"ran env", "HOME=/home/somebody"} {
		if !strings.Contains(history[0].Content, want) {
			t.Errorf("the message lost %q: %q", want, history[0].Content)
		}
	}
}
