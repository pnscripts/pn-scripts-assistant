package server

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * The Learning page: questions go to it without their answers, an answer is
 * marked and the right one comes back only then, and the topic can be
 * looked at and removed.
 */
func TestTheLearningPage(t *testing.T) {
	ts, _, b := newServer(t)

	// No model: a quiz says so plainly rather than failing vaguely.
	b.Router = nil

	if code := sendJSON(t, ts.URL+"/api/study/quiz", map[string]any{"topic": "fractions"}, nil); code != http.StatusServiceUnavailable {
		t.Errorf("a quiz with no model answered %d", code)
	}

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", scribe{reply: `[` +
		`{"kind":"choice","question":"Which is bigger?","options":["1/2","1/3"],"answer":"1/2","explain":"Fewer pieces."},` +
		`{"kind":"number","question":"Half of 9?","answer":"4.5"}]`})
	b.Cfg.DefaultProvider = "ollama"

	var quiz struct {
		Topic struct {
			ID    int64  `json:"id"`
			Level string `json:"level"`
		} `json:"topic"`
		Questions []map[string]any `json:"questions"`
	}

	if code := sendJSON(t, ts.URL+"/api/study/quiz", map[string]any{"topic": "fractions", "level": "child", "count": 2}, &quiz); code != http.StatusOK || len(quiz.Questions) != 2 {
		t.Fatalf("%d %+v", code, quiz)
	}

	for _, q := range quiz.Questions {
		if _, has := q["answer"]; has {
			t.Errorf("a question went to the page with its answer: %v", q)
		}
	}

	if code := sendJSON(t, ts.URL+"/api/study/quiz", map[string]any{"topic": "x", "level": "genius"}, nil); code != http.StatusBadRequest {
		t.Errorf("an unknown level answered %d", code)
	}

	first := int64(quiz.Questions[1]["id"].(float64))

	var marked struct {
		Right    bool   `json:"right"`
		Sure     bool   `json:"sure"`
		Expected string `json:"expected"`
		By       string `json:"by"`
	}

	if code := sendJSON(t, ts.URL+"/api/study/answer", map[string]any{"card": first, "answer": "4,5"}, &marked); code != http.StatusOK ||
		!marked.Right || marked.Expected != "4.5" || marked.By != "rule" {
		t.Errorf("%d %+v", code, marked)
	}

	if code := sendJSON(t, ts.URL+"/api/study/answer", map[string]any{"card": 999, "answer": "x"}, nil); code != http.StatusNotFound {
		t.Errorf("a missing question answered %d", code)
	}

	var list struct {
		Topics []struct {
			Topic string `json:"topic"`
			Cards int    `json:"cards"`
			Right int    `json:"right"`
		} `json:"topics"`
		Due    int              `json:"due"`
		Levels []map[string]any `json:"levels"`
	}

	getJSON(t, ts.URL+"/api/study", &list)

	if len(list.Topics) != 1 || list.Topics[0].Cards != 2 || list.Topics[0].Right != 1 || list.Due != 1 || len(list.Levels) != 4 {
		t.Errorf("%+v", list)
	}

	var review struct {
		Questions []map[string]any `json:"questions"`
		Review    bool             `json:"review"`
	}

	if code := sendJSON(t, ts.URL+"/api/study/quiz", map[string]any{"review": true}, &review); code != http.StatusOK || !review.Review || len(review.Questions) != 1 {
		t.Errorf("%d %+v", code, review)
	}

	id := strconv.FormatInt(quiz.Topic.ID, 10)

	resp, err := http.Get(ts.URL + "/api/study/" + id)
	if err != nil {
		t.Fatal(err)
	}

	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK || strings.Contains(string(page), `"answer"`) || !strings.Contains(string(page), "Half of 9?") {
		t.Errorf("%d %s", resp.StatusCode, page)
	}

	if code := sendJSON(t, ts.URL+"/api/study/"+id+"/remove", nil, nil); code != http.StatusOK {
		t.Errorf("removing answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/study/"+id+"/remove", nil, nil); code != http.StatusNotFound {
		t.Errorf("removing twice answered %d", code)
	}
}

func TestALessonFromThePage(t *testing.T) {
	ts, _, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", scribe{reply: "# Дроби\n## Цел\nДа сравнявате дроби."})
	b.Cfg.DefaultProvider = "ollama"

	var done struct {
		Topic struct {
			Topic  string `json:"topic"`
			Lang   string `json:"lang"`
			Lesson string `json:"lesson"`
		} `json:"topic"`
	}

	if code := sendJSON(t, ts.URL+"/api/study/lesson", map[string]any{"topic": "Дроби", "level": "child"}, &done); code != http.StatusOK ||
		done.Topic.Lang != "bg" || !strings.HasPrefix(done.Topic.Lesson, "# Дроби") {
		t.Errorf("%d %+v", code, done)
	}

	if code := sendJSON(t, ts.URL+"/api/study/lesson", map[string]any{"topic": strings.Repeat("x", 201)}, nil); code != http.StatusBadRequest {
		t.Errorf("a long topic answered %d", code)
	}
}
