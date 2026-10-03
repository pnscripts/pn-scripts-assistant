package brain

import "testing"

// With nobody named, a reminder is simply said. It used to begin ", a
// reminder:", which is read aloud as a pause before the sentence starts.
func TestAReminderWithNoOwnerDoesNotStartWithAComma(t *testing.T) {
	for owner, want := range map[string]string{
		"":    "A reminder: call the dentist",
		"  ":  "A reminder: call the dentist",
		"Sam": "Sam, a reminder: call the dentist",
	} {
		if got := reminderLine(owner, "call the dentist"); got != want {
			t.Errorf("owner %q: said %q, want %q", owner, got, want)
		}
	}
}
