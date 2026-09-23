package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

/*
 * An answer that took longer than the deadline still reaches the person.
 *
 * Measured on this machine before this existed: a first conversation turn
 * that reached for a tool took twenty-eight minutes, the server's
 * fifteen-minute write deadline cut the connection, and the reply was empty —
 * while the assistant had finished the work, written the answer into the
 * conversation and stopped for approval. Everything worked and nobody was
 * told, which is the worst way for something to fail.
 *
 * Tested against a real server with a real deadline, scaled down: the same
 * mechanism, in milliseconds instead of minutes.
 */
func TestAnAnswerThatOutlastsTheDeadlineStillArrives(t *testing.T) {
	const deadline = 150 * time.Millisecond
	const thinking = 600 * time.Millisecond

	slow := func(extend bool) *httptest.Server {
		server := httptest.NewUnstartedServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if extend {
					letItThink(w)
				}

				time.Sleep(thinking)
				w.Write([]byte("the answer"))
			}))

		server.Config.WriteTimeout = deadline
		server.Start()

		return server
	}

	// Without it: the connection is cut and the caller gets nothing.
	server := slow(false)

	resp, err := http.Get(server.URL)
	if err == nil {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr == nil && string(body) == "the answer" {
			t.Skip("this Go runtime did not enforce the write deadline; the " +
				"test below is the one that matters")
		}
	}

	server.Close()

	// With it: the same slow handler answers.
	server = slow(true)
	defer server.Close()

	resp, err = http.Get(server.URL)
	if err != nil {
		t.Fatalf("the slow answer never arrived: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the slow answer: %v", err)
	}

	if string(body) != "the answer" {
		t.Errorf("the answer came back as %q", body)
	}
}

// And the request that waits on a model asks for it. A handler that forgets
// is a handler whose answers disappear on a slow machine, which is the one
// machine this program is written for.
func TestTheHandlersThatWaitOnAModelAskForTheTime(t *testing.T) {
	for _, handler := range []string{"handleChat", "handleSpeak", "handleGreeting"} {
		if !callsLetItThink(t, handler) {
			t.Errorf("%s does not extend its deadline", handler)
		}
	}
}

// callsLetItThink reads the source rather than the behaviour, because the
// behaviour costs a model call. Crude and honest: the alternative is a test
// that passes by never running the thing it is about.
func callsLetItThink(t *testing.T, name string) bool {
	t.Helper()

	body, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}

	from := bytes.Index(body, []byte("func (s *Server) "+name+"("))
	if from < 0 {
		t.Fatalf("%s is not in server.go any more", name)
	}

	// The first thirty lines of it: extending the deadline later than that is
	// extending it after the work has begun.
	end := from
	for lines := 0; lines < 30 && end < len(body); end++ {
		if body[end] == '\n' {
			lines++
		}
	}

	return bytes.Contains(body[from:end], []byte("letItThink(w)"))
}
