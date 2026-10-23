package brain

import (
	"context"

	"pn-scripts-assistant/internal/brain/ask"
	"pn-scripts-assistant/internal/brain/llm"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/route"
)

// installedFor is how long one answer about installed models is reused, so
// a task's steps do not each ask Ollama, while a model pulled or removed is
// still seen within half a minute.
const installedFor = 30 * time.Second

type installedCache struct {
	mu    sync.Mutex
	at    time.Time
	found []route.Candidate

	floorOnce sync.Once
	floor     int
}

/*
 * speedFloor is 1 on a machine where models run on the processor alone, so
 * the router weighs every choice's wait there; 0 with a graphics card.
 */
func (b *Brain) speedFloor() int {
	c := &b.installed

	c.floorOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if models.WhatItCanRun(ctx, models.New(b.Cfg.OllamaURL)).Tier() == models.Modest {
			c.floor = 1
		}
	})

	return c.floor
}

/*
 * installedModels is what the task router chooses from: every model Ollama
 * has on this machine now. Cloud models are not offered here — a step on a
 * hosted service is routed by that service — so nothing chosen this way
 * leaves the computer or costs anything.
 */
func (b *Brain) installedModels() []route.Candidate {
	c := &b.installed

	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.at) < installedFor {
		return append([]route.Candidate{}, c.found...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	list, err := models.New(b.Cfg.OllamaURL).List(ctx)
	if err != nil {
		// Unreachable is not "nothing installed": keep what was known.
		return append([]route.Candidate{}, c.found...)
	}

	found := make([]route.Candidate, 0, len(list))

	for _, m := range list {
		if isEmbedding(m.Name) {
			continue
		}

		found = append(found, route.Candidate{Name: m.Name, Bytes: m.Bytes})
	}

	c.at, c.found = time.Now(), found

	return append([]route.Candidate{}, found...)
}

// isEmbedding is a model that turns text into numbers and cannot answer.
func isEmbedding(name string) bool {
	for _, w := range []string{"embed", "bge-", "minilm", "e5-"} {
		if strings.Contains(name, w) {
			return true
		}
	}

	return false
}

/*
 * answerToQuestion turns a reply to the last question asked in this
 * conversation into the answer it picked: "2", "второто", "the first one" or
 * words from an answer. Anything else — the "Other" of every question — is
 * left exactly as written.
 */
func (b *Brain) answerToQuestion(conversationID int64, reply string) string {
	history, err := b.DB.History(conversationID)
	if err != nil {
		return reply
	}

	for i := len(history) - 1; i >= 0; i-- {
		switch history[i].Role {
		case llm.RoleUser:
			return reply
		case llm.RoleAssistant:
			q, ok := ask.FromShown(history[i].Content)
			if !ok {
				return reply
			}

			if choice, other := ask.Answer(q, reply); !other {
				return choice
			}

			return reply
		}
	}

	return reply
}
