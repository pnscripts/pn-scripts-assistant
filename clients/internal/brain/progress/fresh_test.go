package progress

import "testing"

/*
 * fresh starts a test with no history and nothing on screen, and leaves it
 * that way afterwards.
 *
 * The history and the current step are the program's own globals, so a test
 * that looks at them sees whatever the test before it left there. Run twice
 * (go test -count=2), TestAStepCanBeQuietenedAfterTheFact met the step it had
 * quietened the first time and reported it as quiet before anything had
 * quietened it. Test-only: the program itself never forgets what it did.
 */
func fresh(t *testing.T) {
	t.Helper()

	reset := func() {
		history.mu.Lock()
		history.entries = nil
		history.mu.Unlock()

		current.mu.Lock()
		clearLocked()
		current.round = 0
		current.background = false
		current.model = ""
		current.mu.Unlock()
	}

	reset()
	t.Cleanup(reset)
}
