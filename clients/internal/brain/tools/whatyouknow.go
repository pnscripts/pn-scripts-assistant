package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

/*
 * What is in the memory, answered from the memory rather than by searching it.
 *
 * Recall is a similarity search with a floor under it, and that floor is right:
 * "my PHP projects" finds the PHP projects and nothing else. But the most
 * natural question anybody asks about a memory is the one least like anything
 * in it. "What do you know about me" scored under the floor against all one
 * thousand and twenty-nine stored facts, so nothing was recalled and the model
 * answered from an empty prompt — "I don't have access to personal information
 * about you unless you share it", said by a brain holding a thousand things
 * about the person asking.
 *
 * A tool rather than a phrase this program watches for. The first attempt
 * matched the wording, which meant guessing every way somebody might say it:
 * it caught "what do you know about me", missed "what do you remember", and
 * wrongly claimed "what do you know about PHP" — a specific question that
 * should reach recall and be answered properly. The model already decides when
 * a question needs looking something up; this gives it something to look in.
 */
type WhatYouKnow struct {
	// Memory answers the two questions the store can answer exactly.
	Memory Remembers
}

// Remembers is the part of the store this tool needs.
type Remembers interface {
	CountFacts() (int, error)
	FactsByCategory() (map[string]int, error)
	RecentFacts(limit int) ([]RecentFact, error)
}

// RecentFact is one remembered thing, named for the person reading it.
type RecentFact struct {
	Content  string
	Category string
}

func (WhatYouKnow) Name() string { return "what_you_know" }

func (WhatYouKnow) Description() string {
	return "Report what is in your memory as a whole — how many things you have learned, " +
		"what kinds they are, and a few examples. Use this when asked what you know or " +
		"remember about the person in general, rather than about one particular subject. " +
		"For a specific subject, recall finds it; this answers \"what have you got in there\"."
}

func (WhatYouKnow) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (WhatYouKnow) Risk() Risk { return Safe }

func (WhatYouKnow) Summarize(json.RawMessage) string { return "Look at what you have learned" }

func (t WhatYouKnow) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	if t.Memory == nil {
		return "", fmt.Errorf("there is no memory to look in")
	}

	total, err := t.Memory.CountFacts()
	if err != nil {
		return "", fmt.Errorf("counting what is known: %w", err)
	}

	if total == 0 {
		return "Nothing has been learned yet. Point me at a folder and I will read it.", nil
	}

	var parts []string

	parts = append(parts, fmt.Sprintf("%d things learned.", total))

	/*
	 * The spread, which is the useful half.
	 *
	 * A thousand memories that are all one document folder and a thousand
	 * covering a working life are the same number and a completely different
	 * thing to have.
	 */
	if counts, err := t.Memory.FactsByCategory(); err == nil && len(counts) > 0 {
		type kind struct {
			name string
			n    int
		}

		var kinds []kind

		for name, n := range counts {
			if n > 0 {
				kinds = append(kinds, kind{name, n})
			}
		}

		sort.Slice(kinds, func(i, j int) bool {
			if kinds[i].n != kinds[j].n {
				return kinds[i].n > kinds[j].n
			}

			return kinds[i].name < kinds[j].name
		})

		said := make([]string, 0, len(kinds))

		for _, k := range kinds {
			said = append(said, fmt.Sprintf("%d from %ss", k.n, k.name))
		}

		if len(said) > 0 {
			parts = append(parts, "Mostly "+joinWithAnd(said)+".")
		}
	}

	/*
	 * And a few of them, because a count is not knowledge.
	 *
	 * "I know 1029 things" invites exactly one reply — "such as?" — and
	 * answering it in the same breath is the difference between a report and
	 * an answer.
	 */
	if recent, err := t.Memory.RecentFacts(3); err == nil && len(recent) > 0 {
		said := make([]string, 0, len(recent))

		for _, f := range recent {
			said = append(said, strings.TrimSuffix(firstSentence(f.Content), "."))
		}

		parts = append(parts, "For instance: "+joinWithAnd(said)+".")
	}

	return strings.Join(parts, " "), nil
}

// firstSentence keeps an example to the part that identifies it.
func firstSentence(s string) string {
	if at := strings.IndexAny(s, ".\n"); at > 0 {
		return s[:at+1]
	}

	return s
}

// joinWithAnd writes a list the way it is said aloud, which matters here:
// this answer is read out as often as it is read.
func joinWithAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}
