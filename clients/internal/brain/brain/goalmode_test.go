package brain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/autonomy"
	"pn-scripts-assistant/internal/brain/store"
)

// fakeOllama keeps a list of models, answers what is installed, and deletes.
type fakeOllama struct {
	mu      sync.Mutex
	have    map[string]string
	deleted []string
}

func (f *fakeOllama) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.URL.Path {
	case "/api/tags":
		var list []map[string]any
		for name, digest := range f.have {
			list = append(list, map[string]any{"name": name, "model": name, "digest": digest, "size": 1 << 30})
		}

		json.NewEncoder(w).Encode(map[string]any{"models": list})
	case "/api/delete":
		var body struct{ Model string }
		json.NewDecoder(r.Body).Decode(&body)

		if _, ok := f.have[body.Model]; !ok {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		delete(f.have, body.Model)
		f.deleted = append(f.deleted, body.Model)
	default:
		http.NotFound(w, r)
	}
}

/*
 * Once a day, what goal mode downloaded and nobody used for thirty days is
 * removed — never a model a role still uses, never one it did not download,
 * and nothing at all while a job is under way.
 */
func TestUnusedDownloadsAreRemovedOnTheirOwn(t *testing.T) {
	b := testBrain(t)

	ollama := &fakeOllama{have: map[string]string{
		"qwen3:8b": "sha256:aaa", "qwen2.5-coder:7b": "sha256:bbb", "mistral:7b": "sha256:ccc",
	}}

	srv := httptest.NewServer(http.HandlerFunc(ollama.serve))
	defer srv.Close()

	b.Cfg.OllamaURL = srv.URL
	b.Cfg.OllamaModel = "qwen2.5-coder:7b"

	ledger := autonomy.LedgerAt(b.Root)
	old := time.Now().Add(-45 * 24 * time.Hour)

	for _, name := range []string{"qwen3:8b", "qwen2.5-coder:7b"} {
		if err := ledger.Add(autonomy.Entry{Kind: "model", Name: name, When: old, Used: old}); err != nil {
			t.Fatal(err)
		}
	}

	// While a job is under way nothing is touched.
	id, err := b.DB.NewTask(store.Task{Name: "space game", State: store.TaskWorking})
	if err != nil {
		t.Fatal(err)
	}

	if got := b.tidyOnce(context.Background(), time.Now()); len(got.Removed) != 0 || len(ollama.deleted) != 0 {
		t.Fatalf("removed during work: %+v", got)
	}

	if err := b.DB.FinishTask(id, store.TaskDone, "made", "finished"); err != nil {
		t.Fatal(err)
	}

	got := b.tidyOnce(context.Background(), time.Now())

	if strings.Join(ollama.deleted, ",") != "qwen3:8b" || len(got.Removed) != 1 {
		t.Errorf("deleted %v (%+v)", ollama.deleted, got)
	}

	if _, kept := ollama.have["mistral:7b"]; !kept {
		t.Error("a model goal mode never downloaded was removed")
	}

	if ledger.Mine("model", "qwen3:8b") || !ledger.Mine("model", "qwen2.5-coder:7b") {
		t.Errorf("ledger after: %+v", ledger.Entries())
	}
}
