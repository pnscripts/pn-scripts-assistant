package brain

import (
	"context"
	"testing"

	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * However a question is decided, Doing now stops saying it is waiting.
 *
 * Found on 12 November 2026: after a yes, a no, or the quiet hours answering
 * a held question by itself, the turn's red "Waiting for you: Always asked
 * ..." row stayed until some other work came along. The row is put there
 * the way the loop puts it; the decision goes through Decide or the quiet
 * hours, exactly as the page and the window opening do.
 */
func TestADecisionSettlesItsRowInDoingNow(t *testing.T) {
	cases := []struct {
		name string
		want string
		act  func(*Brain, int64) error
	}{
		{"refused by hand", progress.Refused, func(b *Brain, id int64) error {
			_, err := b.Decide(context.Background(), id, false)
			return err
		}},
		{"approved by hand", progress.Approved, func(b *Brain, id int64) error {
			_, err := b.Decide(context.Background(), id, true)
			return err
		}},
		{"started by itself", progress.ByItself, func(b *Brain, _ int64) error {
			b.SetQuietHours(inside())
			b.AnswerHeldQuestions(context.Background())
			return nil
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var jobs []orchestrator.Job

			b := agentBrain(t, fakeAgent{jobs: &jobs})
			everythingAllowedHere(b)

			inv, _ := heldQuestion(t, b, tools.ReachFolder)

			progress.Done()
			progress.Waiting("Waiting for you: "+inv.Summary, inv.ID)
			progress.Done()

			if err := c.act(b, inv.ID); err != nil {
				t.Fatal(err)
			}

			found := false

			for _, e := range progress.Recent() {
				if e.Kind == "waiting" {
					t.Errorf("Doing now still says it waits: %+v", e)
				}

				if e.Kind == progress.KindDecided && e.Decided == c.want {
					found = true
				}
			}

			if !found {
				t.Errorf("no row says it was %s: %+v", c.want, progress.Recent())
			}
		})
	}
}
