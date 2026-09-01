package brain

import (
	"context"
	"strings"
	"testing"
)

/*
 * A question must never be left in the record with nothing after it.
 *
 * The reply is written when a turn finishes, which is right until a turn does
 * not finish. Learning a folder takes over an hour on this machine, and
 * anything ending the process meanwhile — a restart, a crash, the window
 * closing — stored the question and nothing else. Five conversations were
 * found holding five questions and one answer between them, a transcript
 * saying the brain had been asked things and ignored them.
 */
func TestAnInterruptedTurnStillLeavesAnAnswer(t *testing.T) {
	b := testBrain(t)

	// Cancelled before it can reach a model, which is what a restart looks
	// like from inside a turn.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = b.Chat(ctx, ChatRequest{Message: "learn everything in /tmp"})

	convos, err := b.DB.RecentConversations(5)
	if err != nil || len(convos) == 0 {
		t.Fatalf("no conversation was recorded: %v", err)
	}

	messages, err := b.DB.History(convos[0].ID)
	if err != nil {
		t.Fatalf("could not read it back: %v", err)
	}

	var sawUser, sawAssistant bool

	for _, m := range messages {
		switch m.Role {
		case "user":
			sawUser = true
		case "assistant":
			sawAssistant = true

			/*
			 * Any answer will do, so long as there is one. The learn path
			 * writes its own — "could not finish learning from it" — and the
			 * fallback writes a general note; what must never happen is a
			 * question with nothing after it.
			 */
			if strings.TrimSpace(m.Content) == "" {
				t.Error("an empty answer is the same as none")
			}
		}
	}

	if !sawUser {
		t.Fatal("the question was not recorded at all")
	}

	if !sawAssistant {
		t.Error("the question was left with no answer after it, which is the bug")
	}
}
