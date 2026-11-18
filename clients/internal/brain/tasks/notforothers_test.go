package tasks

import (
	"testing"

	"pn-scripts-assistant/internal/brain/agent"
)

// What a task may not decide for its owner, another program may not
// decide either: the two lists are kept apart (a task's is lifted by
// "everything allowed", another program's never is), and this keeps the
// second from falling behind the first.
func TestWhatIsTheOwnersOwnIsNotAnotherProgramsEither(t *testing.T) {
	for _, tool := range TheOwnersOwn {
		found := false

		for _, each := range agent.NotForAnotherProgram {
			found = found || each == tool
		}

		if !found {
			t.Errorf("%s is the owner's own, and a turn another program asked for may still use it", tool)
		}
	}
}
