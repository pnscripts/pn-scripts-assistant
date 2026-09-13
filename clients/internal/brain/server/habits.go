package server

import (
	"net/http"
	"strconv"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * EnoughToBeAHabit is how many decisions the same way before it is worth
 * mentioning.
 *
 * Four. Three is a run of luck and ten is so many that somebody has already
 * been asked about it nine times more than they wanted — and the whole point
 * of noticing is that a question put repeatedly about something already
 * settled stops being read and starts being clicked through.
 */
const EnoughToBeAHabit = 4

/*
 * handleHabits is what the record of decisions actually says.
 *
 * Every approval and refusal has been written down since the gate was built,
 * and until now nothing read any of it back. Surfaced as a suggestion rather
 * than acted on: widening what the assistant may do without being told to is
 * the one change this program must never make on its own, and narrowing it
 * quietly would be nearly as bad — somebody would be refused something and not
 * know why.
 */
func (s *Server) handleHabits(w http.ResponseWriter, r *http.Request) {
	habits, err := s.brain.DB.Habits()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	standing := map[string]string{}

	if s.brain.Permits != nil {
		for _, granted := range s.brain.Permits.List() {
			standing[granted.Tool] = string(granted.Answer)
		}
	}

	noticed := []map[string]any{}

	for _, h := range habits {
		if !h.Settled(EnoughToBeAHabit) {
			continue
		}

		want := string(permits.Allow)
		if h.Refused > 0 {
			want = string(permits.Refuse)
		}

		// Nothing to suggest about something already decided standing.
		if standing[h.Tool] == want {
			continue
		}

		noticed = append(noticed, map[string]any{
			"tool":     h.Tool,
			"approved": h.Approved,
			"refused":  h.Refused,
			"suggest":  want,
			"said":     saidPlainly(h, want),
		})
	}

	ok(w, map[string]any{"habits": habits, "noticed": noticed})
}

// saidPlainly is the suggestion in the owner's terms rather than the
// program's, because it is a sentence they are being asked to agree with.
func saidPlainly(h store.Habit, want string) string {
	if want == string(permits.Refuse) {
		return "You have said no to this " + times(h.Refused) + " and never yes. " +
			"Always refuse it?"
	}

	return "You have said yes to this " + times(h.Approved) + " and never no. " +
		"Stop asking?"
}

func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	default:
		return strconv.Itoa(n) + " times"
	}
}
