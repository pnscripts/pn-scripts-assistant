package server

import (
	"time"

	"pn-scripts-assistant/internal/brain/quiethours"
)

// quietWait is what a page needs to say, in its own language, that work
// nobody asked for (goals that start themselves, reading places) waits for
// the quiet hours: the window, when it opens, and whether that is tomorrow by
// this machine's clock (a phone in another time zone would count the day
// wrongly). Nil when it does not wait.
func (s *Server) quietWait(now time.Time) map[string]any {
	setting := s.brain.QuietHours()

	waits, opens := quiethours.Waits(setting, now)
	if !waits {
		return nil
	}

	w, _ := quiethours.Parse(setting)

	ny, nm, nd := now.Date()
	oy, om, od := opens.Date()

	return map[string]any{"from": w.From(), "to": w.To(), "opens": opens.Format(time.RFC3339),
		"tomorrow": oy != ny || om != nm || od != nd}
}
