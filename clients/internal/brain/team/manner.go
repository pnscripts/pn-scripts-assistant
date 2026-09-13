package team

import (
	"strings"
)

/*
 * Manner is how somebody works, as a handful of traits rather than a paragraph.
 *
 * Persona stays — a sentence somebody writes about an agent is often the best
 * description of it — and this sits beside it for the part that is better as
 * separate words: how sure of itself it is, how much it says, how far it
 * strays from the obvious, how much risk it will take on, how it decides.
 * Separate, because they are separately worth changing: somebody who wants the
 * writer terser should not have to rewrite who the writer is to get it.
 *
 * Words, not numbers. "Verbosity 0.3" means nothing to the model it is
 * eventually told to, and nothing to the person reading the file; "brief" is
 * the same instruction both can act on.
 *
 * None of it is a memory. These are fictional employees, and the one thing
 * a background must never be is an invented past — "worked at a bank for ten
 * years" is a claim, and a model handed it will repeat it as one. So the
 * background is what it is experienced in, never where or when.
 */
type Manner struct {
	Confidence    string   `json:"confidence,omitempty"`     // cautious, measured, confident
	Verbosity     string   `json:"verbosity,omitempty"`      // brief, ordinary, thorough
	Creativity    string   `json:"creativity,omitempty"`     // conventional, balanced, inventive
	RiskTolerance string   `json:"risk_tolerance,omitempty"` // careful, balanced, bold
	Decides       string   `json:"decides,omitempty"`        // by evidence, by consensus, quickly
	Speaks        string   `json:"speaks,omitempty"`         // plain, formal, warm
	Background    string   `json:"background,omitempty"`     // what it is experienced in
	Strengths     []string `json:"strengths,omitempty"`
	Weaknesses    []string `json:"weaknesses,omitempty"`
}

// Empty reports whether nothing has been said about how this one works.
func (m Manner) Empty() bool {
	return m.Confidence == "" && m.Verbosity == "" && m.Creativity == "" &&
		m.RiskTolerance == "" && m.Decides == "" && m.Speaks == "" &&
		m.Background == "" && len(m.Strengths) == 0 && len(m.Weaknesses) == 0
}

/*
 * Told is the manner as one instruction a model can follow.
 *
 * One sentence of parts joined, because a list of labelled traits reads to a
 * small model as a form to fill in, and it answers by describing itself. The
 * weaknesses are said as things to watch for rather than as faults — a model
 * told it is "bad at estimates" produces bad estimates with an apology.
 */
func (m Manner) Told() string {
	var parts []string

	add := func(format, value string) {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, strings.ReplaceAll(format, "%", value))
		}
	}

	add("you are % in how sure you sound", m.Confidence)
	add("your answers are %", m.Verbosity)
	add("your ideas are %", m.Creativity)
	add("you are % about risk", m.RiskTolerance)
	add("you decide %", m.Decides)
	add("you speak in a % way", m.Speaks)
	add("you are experienced in %", m.Background)

	if len(m.Strengths) > 0 {
		parts = append(parts, "you are strongest at "+strings.Join(m.Strengths, ", "))
	}

	if len(m.Weaknesses) > 0 {
		parts = append(parts, "watch yourself on "+strings.Join(m.Weaknesses, ", "))
	}

	if len(parts) == 0 {
		return ""
	}

	told := strings.Join(parts, "; ")

	return strings.ToUpper(told[:1]) + told[1:] + "."
}

// mannerKeys are the file keys, in the order they are written.
var mannerKeys = []string{
	"confidence", "verbosity", "creativity", "risk_tolerance", "decides", "speaks",
	"background", "strengths", "weaknesses",
}

// read takes one key from a file into the manner, and says whether it was one.
func (m *Manner) read(key, value string) bool {
	switch key {
	case "confidence":
		m.Confidence = value
	case "verbosity":
		m.Verbosity = value
	case "creativity":
		m.Creativity = value
	case "risk_tolerance", "risk":
		m.RiskTolerance = value
	case "decides":
		m.Decides = value
	case "speaks":
		m.Speaks = value
	case "background":
		m.Background = value
	case "strengths":
		m.Strengths = commaList(value)
	case "weaknesses":
		m.Weaknesses = commaList(value)
	default:
		return false
	}

	return true
}

// written is the manner as file lines, in mannerKeys order, empties left out.
func (m Manner) written() [][2]string {
	values := map[string]string{
		"confidence": m.Confidence, "verbosity": m.Verbosity, "creativity": m.Creativity,
		"risk_tolerance": m.RiskTolerance, "decides": m.Decides, "speaks": m.Speaks,
		"background": m.Background, "strengths": strings.Join(m.Strengths, ", "),
		"weaknesses": strings.Join(m.Weaknesses, ", "),
	}

	out := [][2]string{}

	for _, key := range mannerKeys {
		if v := strings.TrimSpace(values[key]); v != "" {
			out = append(out, [2]string{key, v})
		}
	}

	return out
}

// commaList splits on commas only: a strength is often several words, where
// the tool and capability lists split on spaces too.
func commaList(value string) []string {
	var out []string

	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}
