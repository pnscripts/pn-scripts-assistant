package team

import (
	"strings"

	"pn-scripts-assistant/internal/brain/route"
)

// codingWords in a title, job or capability mean the work is writing code.
var codingWords = []string{"program", "developer", "coder", "software", "code", "програм", "разработчик"}

/*
 * RouteNeeds is what this one's work asks of a model, in the terms the router
 * chooses by.
 *
 * Built from what the agent already says about itself — the role it uses,
 * its needs, its title, job and capabilities — so no roster file has to be
 * rewritten for the router to choose well, and an agent that names a model
 * (Prefers) never reaches the router at all.
 */
func (a Agent) RouteNeeds() route.Needs {
	n := route.Needs{Tools: a.Needs.Tools, Vision: a.Needs.Vision, Offline: a.Needs.Local}

	switch a.Uses {
	case UsesQuick:
		n.Speed = 3
	case UsesTalk:
		n.Speed = 2
	case UsesBest:
		n.Reasoning = 2
	case UsesReason:
		n.Reasoning = 3
	default:
		n.Tools = true
		n.Reasoning = 1
	}

	if a.Needs.Thinks {
		n.Reasoning = 3
	}

	about := strings.ToLower(a.Title + " " + a.Job + " " + a.Position + " " + strings.Join(a.Can, " "))
	for _, w := range codingWords {
		if strings.Contains(about, w) {
			n.Coding = 3

			break
		}
	}

	return n
}
