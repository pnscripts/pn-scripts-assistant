package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

/*
 * A dropped connection should cost a second, not a conversation.
 *
 * There was no retry anywhere: one transient failure — a laptop changing
 * network, a hosted service shedding load, Ollama still binding its port —
 * ended the turn with an error and the question had to be asked again. Work
 * in the background survived it by failing over to somebody else; a
 * conversation has nowhere to fail over to.
 */
func TestAFailedFirstAttemptIsTriedOnceMore(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// What a service under load says.
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "the answer"},
			"done":    true,
		})
	}))
	defer server.Close()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	resp, err := o.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("the second attempt was never made: %v", err)
	}

	if resp.Content != "the answer" {
		t.Errorf("the answer came back as %q", resp.Content)
	}

	if got := attempts.Load(); got != 2 {
		t.Errorf("it made %d attempts, not 2", got)
	}
}

// And twice is the end of it: a service that is down is reported as down
// rather than hammered until somebody notices.
func TestItDoesNotKeepTrying(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	if _, err := o.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}); err == nil {
		t.Fatal("a service answering 502 twice was reported as working")
	}

	if got := attempts.Load(); got != Tries {
		t.Errorf("it made %d attempts, not %d", got, Tries)
	}
}

/*
 * A refusal is not a failure to be retried.
 *
 * Asking the same wrong question twice gets the same answer twice, a second
 * later. Worse for a paid service: two charges for one mistake.
 */
func TestARefusalIsAskedOnlyOnce(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"that model does not exist"}`))
	}))
	defer server.Close()

	o := NewOllama(server.URL, "nothing-like-this", "nomic-embed-text")

	if _, err := o.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}); err == nil {
		t.Fatal("a 400 was treated as success")
	}

	if got := attempts.Load(); got != 1 {
		t.Errorf("a refusal was asked %d times", got)
	}
}

// Somebody who stopped waiting is not asked again on their behalf.
func TestSomethingCancelledIsNotRetried(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	ctx, stop := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer stop()

	o := NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	if _, err := o.Chat(ctx, Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}); err == nil {
		t.Fatal("a cancelled call reported success")
	}

	if got := attempts.Load(); got != 1 {
		t.Errorf("a cancelled call was made %d times", got)
	}
}

// The service's own "try in a moment" is honoured, and bounded: a day is not
// a retry, it is a different conversation.
func TestItWaitsAsLongAsItWasAskedTo(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}

	resp.Header.Set("Retry-After", "2")

	if got := whenItSaidToTry(resp); got != 2*time.Second {
		t.Errorf("it waited %v when told two seconds", got)
	}

	resp.Header.Set("Retry-After", "86400")

	if got := whenItSaidToTry(resp); got != LongestWait {
		t.Errorf("it would have waited %v", got)
	}

	resp.Header.Set("Retry-After", "sometime")

	if got := whenItSaidToTry(resp); got != 0 {
		t.Errorf("nonsense became a wait of %v", got)
	}
}
