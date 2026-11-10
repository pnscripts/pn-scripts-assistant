package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
)

/*
 * What the terminal client says is said under the program's name.
 *
 * The client fell back to calling itself "brain" and asked "Is the brain
 * running?", so every answer in a terminal read "brain [ollama/…]> …". The
 * program is PN Scripts Assistant (owner, 10.11.2026). These run the client
 * the way a person does, against a fake assistant, and read what it printed.
 */

// brainWord is "brain" as a word of its own, in any case.
var brainWord = regexp.MustCompile(`(?i)\bbrain\b`)

// isolated gives the client a scratch config folder and no address from the
// environment, so a test never reads or writes the conversation of a real
// terminal on this machine.
func isolated(t *testing.T) {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv(apiURLEnv, "")
	t.Setenv(legacyAPIURLEnv, "")
}

// fakeAssistant answers /api/status with a name, and /api/chat with a reply.
func fakeAssistant(t *testing.T, name string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "owner": "Sam"})
	})
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conversation_id": 7, "reply": "hello", "provider": "ollama", "model": "m",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func runCLI(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()

	var out, errs bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errs)

	return out.String(), errs.String(), code
}

func TestTheHelpNamesTheProgramNotTheBrain(t *testing.T) {
	isolated(t)

	_, help, code := runCLI(t, "", "-h")
	if code != 0 {
		t.Fatalf("-h exited %d", code)
	}

	if brainWord.MatchString(help) {
		t.Errorf("the help still calls something the brain:\n%s", help)
	}

	for _, want := range []string{"PN Scripts Assistant", apiURLEnv} {
		if !strings.Contains(help, want) {
			t.Errorf("the help does not mention %q:\n%s", want, help)
		}
	}
}

func TestWithNothingAnsweringItSaysWhatIsNotRunning(t *testing.T) {
	isolated(t)

	// A port nothing listens on: a server started and closed at once.
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()

	out, _, _ := runCLI(t, "exit\n", "--api", dead.URL)

	if brainWord.MatchString(out) {
		t.Errorf("with nothing answering it still talks about the brain:\n%s", out)
	}

	if !strings.Contains(out, "Is PN Scripts Assistant running?") {
		t.Errorf("it does not ask whether PN Scripts Assistant is running:\n%s", out)
	}

	if !strings.Contains(out, "pn-scripts-assistant") {
		t.Errorf("it does not say which command starts it:\n%s", out)
	}
}

func TestAnAnswerIsSignedWithTheAssistantsName(t *testing.T) {
	isolated(t)

	srv := fakeAssistant(t, "Alex")

	out, _, code := runCLI(t, "", "--api", srv.URL, "hi")
	if code != 0 {
		t.Fatalf("exited %d", code)
	}

	if !strings.HasPrefix(out, "alex [ollama/m]> hello") {
		t.Errorf("the answer is not signed with the assistant's name: %q", out)
	}
}

func TestAnAnswerFromAnUnnamedAssistantIsSignedWithTheProgramsName(t *testing.T) {
	isolated(t)

	// An assistant that has not been named yet, or an older one that does not
	// say its name: the program's own command, never "brain".
	srv := fakeAssistant(t, "")

	out, _, _ := runCLI(t, "", "--api", srv.URL, "hi")

	if !strings.HasPrefix(out, "pn-scripts-assistant [ollama/m]> hello") {
		t.Errorf("an unnamed assistant's answer is not signed pn-scripts-assistant: %q", out)
	}
}

func TestTheGreetingUsesWhatTheAssistantSaysAboutItself(t *testing.T) {
	isolated(t)

	srv := fakeAssistant(t, "Alex")

	out, _, _ := runCLI(t, "exit\n", "--api", srv.URL)

	if !strings.HasPrefix(out, "Alex online. How can I help, Sam?") {
		t.Errorf("the greeting did not come from /api/status: %q", out)
	}
}

// The address it talks to by default is the one the program serves on by
// default. It was 8090 while the program served on 8790, so the client found
// nothing unless it was told where to look.
func TestByDefaultItTalksToWhereTheProgramServes(t *testing.T) {
	isolated(t)

	if want := "http://" + config.Default().Addr; defaultAPIURL != want {
		t.Errorf("default address %s, the program serves on %s", defaultAPIURL, want)
	}

	if got := apiURLFromEnv(); got != defaultAPIURL {
		t.Errorf("with nothing set it talks to %s", got)
	}
}

func TestTheAddressIsSetUnderTheProgramsNameAndTheOldNameStillWorks(t *testing.T) {
	isolated(t)

	t.Setenv(legacyAPIURLEnv, "http://127.0.0.1:1111")
	if got := apiURLFromEnv(); got != "http://127.0.0.1:1111" {
		t.Errorf("BRAIN_API_URL alone is no longer read: %s", got)
	}

	t.Setenv(apiURLEnv, "http://127.0.0.1:2222")
	if got := apiURLFromEnv(); got != "http://127.0.0.1:2222" {
		t.Errorf("PN_SCRIPTS_ASSISTANT_API_URL does not win over BRAIN_API_URL: %s", got)
	}

	if apiURLEnv != "PN_SCRIPTS_ASSISTANT_API_URL" || legacyAPIURLEnv != "BRAIN_API_URL" {
		t.Errorf("the variables are %s and %s", apiURLEnv, legacyAPIURLEnv)
	}
}

// An assistant nobody has told whose it is greets without an empty name.
func TestTheGreetingDoesNotAddressNobody(t *testing.T) {
	isolated(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "Assistant", "owner": ""})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, _, _ := runCLI(t, "exit\n", "--api", srv.URL)

	if !strings.HasPrefix(out, "Assistant online. How can I help? (") {
		t.Errorf("greeting: %q", out)
	}
}
