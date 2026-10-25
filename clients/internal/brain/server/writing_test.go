package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

// scribe is a model that always rewrites to the same text.
type scribe struct{ reply string }

func (scribe) Name() string                                     { return "ollama" }
func (scribe) Available(context.Context) bool                   { return true }
func (scribe) EmbedModel() string                               { return "same" }
func (scribe) Embed(context.Context, string) ([]float32, error) { return []float32{1}, nil }
func (s scribe) Chat(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Content: s.reply}, nil
}

func TestTheWritingPageProofreadsAppliesAndCompares(t *testing.T) {
	ts, _, _ := newServer(t)

	var report struct {
		Language string `json:"language"`
		Findings []struct {
			Rule string `json:"rule"`
			Sure bool   `json:"sure"`
		} `json:"findings"`
		Tone      []struct{ Rule string } `json:"tone"`
		Corrected string                  `json:"corrected"`
	}

	code := sendJSON(t, ts.URL+"/api/writing/check", map[string]any{
		"text": "Живея в Варна. Мисля че е прав!", "tone": "formal",
	}, &report)

	if code != http.StatusOK || report.Language != "bg" || len(report.Findings) != 2 ||
		report.Corrected != "Живея във Варна. Мисля, че е прав!" || len(report.Tone) != 1 {
		t.Fatalf("%d %+v", code, report)
	}

	var applied struct{ Text string }

	sendJSON(t, ts.URL+"/api/writing/apply", map[string]any{"text": "Done:Next", "chosen": []int{0}}, &applied)

	if applied.Text != "Done: Next" {
		t.Errorf("applied as %q", applied.Text)
	}

	var diff struct {
		Parts []struct{ Kind, Text string } `json:"parts"`
		Added int                           `json:"words_added"`
	}

	sendJSON(t, ts.URL+"/api/writing/compare", map[string]any{"before": "a b c", "after": "a x c"}, &diff)

	if diff.Added != 1 || len(diff.Parts) != 4 {
		t.Errorf("%+v", diff)
	}

	var tones struct {
		Tones []struct {
			Name    string `json:"name"`
			LabelBG string `json:"label_bg"`
		} `json:"tones"`
	}

	getJSON(t, ts.URL+"/api/writing/tones", &tones)

	if len(tones.Tones) != 6 || tones.Tones[0].LabelBG == "" {
		t.Errorf("%+v", tones)
	}

	if code := sendJSON(t, ts.URL+"/api/writing/check", map[string]any{"text": "  "}, nil); code != http.StatusBadRequest {
		t.Errorf("empty text answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/writing/check", map[string]any{"text": strings.Repeat("я", mostText+1)}, nil); code != http.StatusRequestEntityTooLarge {
		t.Errorf("an over-long text answered %d", code)
	}
}

/*
 * A rewrite comes back proofread and compared with what was given, and an
 * unknown tone is refused before any model is asked.
 */
func TestARewriteIsCheckedAndCompared(t *testing.T) {
	ts, _, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", scribe{reply: "```\nHere is the text:\nДобър ден. Моля, изпратете договора!\n```"})
	b.Cfg.DefaultProvider = "ollama"

	var done struct {
		Text   string `json:"text"`
		Report struct {
			Tone []struct{ Rule string } `json:"tone"`
		} `json:"report"`
		Diff struct {
			Added int `json:"words_added"`
		} `json:"diff"`
	}

	code := sendJSON(t, ts.URL+"/api/writing/rewrite", map[string]any{"text": "Здрасти, прати договора.", "tone": "formal"}, &done)

	if code != http.StatusOK || done.Text != "Добър ден. Моля, изпратете договора!" {
		t.Fatalf("%d %+v", code, done)
	}

	if len(done.Report.Tone) != 1 || done.Report.Tone[0].Rule != "exclamation" || done.Diff.Added == 0 {
		t.Errorf("the rewrite was not checked: %+v", done)
	}

	if code := sendJSON(t, ts.URL+"/api/writing/rewrite", map[string]any{"text": "x", "tone": "angry"}, nil); code != http.StatusBadRequest {
		t.Errorf("an unknown tone answered %d", code)
	}
}
