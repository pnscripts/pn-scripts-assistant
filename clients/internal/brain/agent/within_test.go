package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
)

// writing is a tool that writes where its path says, and says so.
type writing struct{ counted }

func (writing) Touches(args json.RawMessage) ([]string, []string) {
	var a struct{ Path string }

	json.Unmarshal(args, &a)

	return []string{a.Path}, nil
}

// project allows one folder.
type project string

func (p project) Allows(path string) bool { return strings.HasPrefix(path, string(p)+"/") }
func (p project) Output(path string) bool { return strings.HasPrefix(path, string(p)+"/build/") }
func (p project) Writable() []string      { return []string{string(p)} }

func writingTo(path string) *scripted {
	return &scripted{replies: []llm.Response{{
		Content:   "Writing it.",
		ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write_it", Arguments: json.RawMessage(`{"path":"` + path + `"}`)}},
	}}}
}

/*
 * Work on a project cannot write outside it — refused, not put to its owner,
 * however freely everything else is allowed.
 */
func TestWorkOnAProjectStaysInTheProject(t *testing.T) {
	for path, allowed := range map[string]bool{
		"/home/p/tetris/src/game.js": true,
		"/home/p/.bashrc":            false,
		"/etc/hosts":                 false,
	} {
		ran := 0

		loop, db := newLoop(t, writing{counted{name: "write_it", ran: &ran}})
		loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

		conv, _ := db.NewConversation("t")

		res, err := loop.RunBrief(context.Background(), conv, writingTo(path), nil,
			Brief{WithTools: true, Within: project("/home/p/tetris")})
		if err != nil {
			t.Fatal(err)
		}

		if (ran == 1) != allowed {
			t.Errorf("%s: ran %d times, allowed %v", path, ran, allowed)
		}

		if !allowed && (len(res.Pending) != 0 || len(res.Steps) != 1 || !res.Steps[0].Failed) {
			t.Errorf("%s: an outside write was put to its owner or not recorded: %+v", path, res)
		}
	}
}
