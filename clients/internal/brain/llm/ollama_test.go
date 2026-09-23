package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

/*
 * Only the models that have a thinking mode are told to skip it.
 *
 * Sending "think" to one without it is rejected outright, so a blanket default
 * would break every other model in order to help this one.
 */
func TestOnlyThinkingModelsAreAskedToBeQuiet(t *testing.T) {
	for _, model := range []string{"qwen3:8b", "qwen3", "deepseek-r1:8b", "qwq:32b"} {
		got := quietThinking(model)

		if got == nil {
			t.Errorf("%s thinks out loud and was not asked not to", model)

			continue
		}

		if *got {
			t.Errorf("%s was asked to think, which is the opposite", model)
		}
	}

	for _, model := range []string{"qwen2.5-coder:7b", "llama3.2:3b", "gemma3:4b"} {
		if quietThinking(model) != nil {
			t.Errorf("%s has no thinking mode and would reject the field", model)
		}
	}
}

/*
 * The one dependency this program cannot work without says what to do when it
 * is not there.
 *
 * "connection refused" is accurate and useless. The address is in the message
 * because it is configurable, and half of these are a brain pointed at the
 * wrong one.
 */
func TestItSaysWhatToDoWhenOllamaIsNotThere(t *testing.T) {
	o := NewOllama("http://127.0.0.1:1", "qwen", "nomic")

	_, err := o.Chat(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}})
	if err == nil {
		t.Fatal("something answered on a port nothing listens on")
	}

	for _, want := range []string{"Ollama is not answering", "http://127.0.0.1:1", "ollama serve", "OLLAMA_BASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not say %q:\n%s", want, err)
		}
	}
}

/*
 * The model is given room for the whole turn, not the end of it.
 *
 * Measured on this machine before this existed: a turn of persona and
 * thirty-nine tools came to 6,338 tokens, Ollama admitted the last 2,050 of
 * them at its default window, and the model — having never been given the
 * instructions — called the same tool eight times and gave up. Nothing in the
 * request failed; the answer was just wrong, which is why this is tested
 * rather than left to be noticed again.
 */
func TestTheWindowIsSizedToTheTurn(t *testing.T) {
	var asked map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&asked)
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "hello"},
			"done":    true,
		})
	}))
	defer server.Close()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	// A turn about the size of a real one: a long persona and a pile of tool
	// schemas.
	req := Request{
		Messages: []Message{
			{Role: RoleSystem, Content: strings.Repeat("who you are and how to behave. ", 200)},
			{Role: RoleUser, Content: "say hello"},
		},
	}

	for i := 0; i < 39; i++ {
		req.Tools = append(req.Tools, ToolSpec{
			Name:        fmt.Sprintf("tool_%d", i),
			Description: strings.Repeat("what this tool does and when to use it. ", 10),
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		})
	}

	if _, err := o.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	options, _ := asked["options"].(map[string]any)

	window, _ := options["num_ctx"].(float64)
	if window == 0 {
		t.Fatalf("no window was asked for; Ollama would truncate this turn: %v", asked["options"])
	}

	// It has to be big enough for the turn, or the beginning is thrown away.
	size := len(req.Messages[0].Content) + len(req.Messages[1].Content)

	for _, tool := range req.Tools {
		size += len(tool.Name) + len(tool.Description) + len(tool.Parameters)
	}

	if int(window)*4 < size {
		t.Errorf("the window is %d tokens for about %d bytes of turn", int(window), size)
	}

	if int(window) > MostRoom {
		t.Errorf("it asked for %d tokens, beyond the cap of %d", int(window), MostRoom)
	}

	// Quantised, so that a turn one word longer does not make Ollama load the
	// model again.
	if int(window)%RoomStep != 0 {
		t.Errorf("the window %d is not a round size", int(window))
	}
}

// A short turn is not given a large window: the memory is real and the
// machine this runs on has no graphics card to put it in.
func TestASmallTurnKeepsTheOrdinaryWindow(t *testing.T) {
	var asked map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&asked)
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "hello"},
			"done":    true,
		})
	}))
	defer server.Close()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	if _, err := o.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}

	options, _ := asked["options"].(map[string]any)

	if window, _ := options["num_ctx"].(float64); int(window) != LeastRoom {
		t.Errorf("a five-word turn asked for %d tokens", int(window))
	}
}

// A window is memory, so a small machine is not asked for a large one.
func TestTheWindowFitsTheMachine(t *testing.T) {
	for _, c := range []struct {
		ram  uint64
		want int
	}{
		{32 << 30, MostRoom},
		{16 << 30, MostRoom},
		{8 << 30, 8192},
		{4 << 30, LeastRoom},
		{0, 8192}, // nothing measured
	} {
		if got := RoomFor(c.ram); got != c.want {
			t.Errorf("%d GB of memory was offered %d tokens, not %d", c.ram>>30, got, c.want)
		}
	}
}

// The cap holds: a turn bigger than the machine asks for what the machine has
// and no more. The model then sees the end of the turn rather than nothing at
// all, which is Ollama's behaviour and not something this can undo.
func TestABigTurnOnASmallMachineStopsAtTheCap(t *testing.T) {
	var asked map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&asked)
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "hello"},
			"done":    true,
		})
	}))
	defer server.Close()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")
	o.MostRoom = LeastRoom

	if _, err := o.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleSystem, Content: strings.Repeat("a long instruction. ", 5000)}},
	}); err != nil {
		t.Fatal(err)
	}

	options, _ := asked["options"].(map[string]any)

	if window, _ := options["num_ctx"].(float64); int(window) != LeastRoom {
		t.Errorf("it asked for %d tokens on a machine capped at %d", int(window), LeastRoom)
	}
}

/*
 * The window does not shrink back between turns.
 *
 * Ollama loads the model again whenever the window changes and keeps what it
 * has read only while it stays the same. A big conversation turn followed by
 * a small housekeeping call would otherwise reload it twice and make the next
 * turn read every token again — which on a machine with no graphics card is
 * minutes, for a call that asked for two hundred tokens.
 */
func TestTheWindowDoesNotShrinkBackBetweenTurns(t *testing.T) {
	var windows []int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var asked map[string]any

		json.NewDecoder(r.Body).Decode(&asked)

		options, _ := asked["options"].(map[string]any)
		window, _ := options["num_ctx"].(float64)
		windows = append(windows, int(window))

		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "ok"},
			"done":    true,
		})
	}))
	defer server.Close()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	big := Request{Messages: []Message{
		{Role: RoleSystem, Content: strings.Repeat("who you are and what to do. ", 900)},
		{Role: RoleUser, Content: "say hello"},
	}}

	small := Request{Messages: []Message{{Role: RoleSystem, Content: "extract the facts"}}}

	for _, req := range []Request{big, small, small} {
		if _, err := o.Chat(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}

	if len(windows) != 3 {
		t.Fatalf("expected three requests, saw %d", len(windows))
	}

	if windows[0] <= LeastRoom {
		t.Fatalf("the big turn asked for only %d tokens", windows[0])
	}

	for i, window := range windows {
		if window != windows[0] {
			t.Errorf("request %d asked for %d after %d was already asked for",
				i+1, window, windows[0])
		}
	}

	// A different model is its own window: it is a different process with its
	// own memory, and holding it open at another model's size buys nothing.
	if _, err := o.Chat(context.Background(), Request{
		Model:    "llama3.2:3b",
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}

	if windows[3] != LeastRoom {
		t.Errorf("another model inherited a window of %d", windows[3])
	}
}
