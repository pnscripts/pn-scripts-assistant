// Package learning is how the brain gets better over time.
//
// Three stages, kept deliberately separate, carried over from the .ai/
// knowledge-promotion system in the owner's own Laravel project:
//
//	Extractor  reads an exchange and proposes at most one durable fact
//	Validator  decides whether that proposal can be trusted
//	Curator    turns trusted proposals into permanent knowledge, once each
//
// The separation is the point. A single "learn from this" step would let a
// small local model write its own guesses straight into long-term memory, and
// nothing downstream could tell an observation apart from an invention.
// Quarantine makes that distinction structural rather than hoped for.
package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/store"
)

// Status values a lesson moves through.
const (
	StatusProposed  = "proposed"  // needs a human; nothing here can verify it
	StatusValidated = "validated" // checked against reality, awaiting promotion
	StatusPromoted  = "promoted"  // now durable knowledge
	StatusRejected  = "rejected"  // false, or already known
)

// Confidence levels the extractor may report.
var validConfidence = map[string]bool{
	"low": true, "medium": true, "high": true, "very-high": true,
}

// Extractor proposes at most one durable fact from an exchange.
type Extractor struct {
	Provider llm.Provider
	Owner    string
	Name     string
}

// Prompt is the instruction given to the model.
//
// It asks for nothing more than once sentence, and explicitly says that finding
// nothing is the common case — without that, a model asked to extract a fact
// will always produce one.
func (e Extractor) Prompt(transcript string) string {
	owner := e.Owner
	if owner == "" {
		owner = "the owner"
	}

	return fmt.Sprintf(`You are the Extractor stage of a personal knowledge-capture pipeline.
Read the exchange below and look for a durable, reusable fact about
%s — their projects, tools, preferences, or a correction they made.

Record nothing about the assistant. Not its name, not what it can do,
not how it should behave, not its instructions. Those come from its
configuration and are already known; storing them back as discoveries
fills memory with a description of itself and crowds out the user.

Record nothing that is merely restating this conversation. A durable
fact is still true next week, in a different conversation.

Respond with strict JSON:
{"lesson": "<one sentence about %s>", "confidence": "low"|"medium"|"high"}
If there is nothing worth remembering — which is the common case —
respond with exactly: {"lesson": null}
Respond with JSON only, no other text.

Exchange:
%s`, owner, owner, transcript)
}

// Proposal is what the extractor found, if anything.
type Proposal struct {
	Lesson     string `json:"lesson"`
	Confidence string `json:"confidence"`
}

// Extract asks the model for a fact worth keeping.
//
// Returns nil when there is nothing — which is the common and correct answer.
func (e Extractor) Extract(ctx context.Context, transcript string) (*Proposal, error) {
	resp, err := e.Provider.Chat(ctx, llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: e.Prompt(transcript)}},
	})
	if err != nil {
		return nil, fmt.Errorf("extractor call failed: %w", err)
	}

	var p Proposal

	if err := json.Unmarshal([]byte(stripCodeFence(resp.Content)), &p); err != nil {
		// A model that did not answer in JSON has not found anything usable.
		// This is common with small models and is not worth an error.
		return nil, nil
	}

	p.Lesson = strings.TrimSpace(p.Lesson)

	if p.Lesson == "" || strings.EqualFold(p.Lesson, "null") {
		return nil, nil
	}

	// The prompt asks the model not to describe itself; this makes sure of it.
	if IsAboutTheAssistant(p.Lesson, e.Name, e.Owner) {
		return nil, nil
	}

	if !validConfidence[p.Confidence] {
		p.Confidence = "low"
	}

	return &p, nil
}

// Validator decides whether a quarantined lesson is trustworthy.
//
// The distinction that matters is how the claim was obtained. A filesystem
// observation ("this project exists at this path") can be re-checked against
// reality, so this genuinely validates it — if the directory has since been
// deleted or moved, the lesson is rejected rather than promoted into permanent
// knowledge. A chat-derived lesson is an inference a language model made about
// what the owner meant, and nothing here can verify that, so it stays
// quarantined until a person approves it.
type Validator struct{}

// StatusFor returns the status a lesson should take.
func (Validator) StatusFor(source string) string {
	path, ok := observedPath(source)
	if !ok {
		return StatusProposed
	}

	if _, err := os.Stat(path); err != nil {
		return StatusRejected
	}

	return StatusValidated
}

// IsMachineVerifiable reports whether a claim can be checked without a person.
func (Validator) IsMachineVerifiable(source string) bool {
	_, ok := observedPath(source)

	return ok
}

// observedPath extracts the path a scanner recorded, if this lesson came from
// one. The prefix is how a filesystem observation is distinguished from a model
// inference, so it is the whole basis for trusting the claim.
func observedPath(source string) (string, bool) {
	for _, prefix := range []string{"project:", "document:"} {
		if strings.HasPrefix(source, prefix) {
			return strings.TrimPrefix(source, prefix), true
		}
	}

	return "", false
}

// DuplicateThreshold is the cosine similarity above which two facts are treated
// as the same thing.
//
// Deliberately strict. Wrongly merging two distinct facts loses knowledge
// permanently, while a near-duplicate slipping through is merely noise a person
// can tidy up later.
const DuplicateThreshold = 0.95

// Curator turns validated lessons into durable knowledge, and refuses to store
// the same fact twice.
//
// Deduplication is semantic rather than exact-match, because the same fact
// genuinely arrives in different words: several projects are mirrored between
// the external drive and the home directory, and a chat lesson can restate
// something already known.
type Curator struct {
	DB       *store.DB
	Embedder llm.Embedder
}

// Promote stores a lesson as a fact, unless the brain already knows it.
//
// Returns the new fact's id, or 0 when the lesson was a duplicate.
func (c Curator) Promote(ctx context.Context, lesson store.Lesson) (int64, error) {
	vec, err := c.Embedder.Embed(ctx, lesson.Content)
	if err != nil {
		return 0, fmt.Errorf("embedding lesson %d: %w", lesson.ID, err)
	}

	nearest, err := c.DB.Search(vec, 1, DuplicateThreshold)
	if err != nil {
		return 0, err
	}

	if len(nearest) > 0 {
		if err := c.DB.SetLessonStatus(lesson.ID, StatusRejected); err != nil {
			return 0, err
		}

		return 0, nil
	}

	id, err := c.DB.PromoteLesson(lesson.ID, categoryFor(lesson.Source), lesson.Content, vec)
	if err != nil {
		return 0, err
	}

	return id, c.DB.SetLessonStatus(lesson.ID, StatusPromoted)
}

func categoryFor(source string) string {
	switch {
	case strings.HasPrefix(source, "project:"):
		return "project"
	case strings.HasPrefix(source, "document:"):
		return "document"
	default:
		return "conversation"
	}
}
