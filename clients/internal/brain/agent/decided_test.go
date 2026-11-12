package agent

import (
	"testing"

	"pn-scripts-assistant/internal/brain/progress"
)

/*
 * The row a question leaves in Doing now knows which question it is, so the
 * decision, whenever and however it comes, settles that row.
 */
func TestTheWaitingRowBelongsToItsQuestion(t *testing.T) {
	progress.Done()

	pending, _, _ := heldCall(t, `{"task":"Tidy the tests.","folder":"/tmp/project","access":"full"}`,
		everythingAllowed)

	if !progress.Decided(pending.ID, progress.Refused) {
		t.Fatalf("the loop's waiting row does not name question %d: %+v", pending.ID, progress.Recent())
	}

	for _, e := range progress.Recent() {
		if e.Kind == "waiting" {
			t.Errorf("still waiting after it was refused: %+v", e)
		}
	}
}
