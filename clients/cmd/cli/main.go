// pn-scripts-assistant-cli is a terminal client for PN Scripts Assistant: it
// asks the running program over its local HTTP API and prints the answers. It
// keeps no knowledge of its own; everything it says comes from the program
// running on this machine.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/release"
)

type cliState struct {
	ConversationID *int `json:"conversation_id,omitempty"`
}

type chatRequest struct {
	ConversationID *int    `json:"conversation_id,omitempty"`
	Message        string  `json:"message"`
	Provider       *string `json:"provider,omitempty"`
}

type chatResponse struct {
	ConversationID int    `json:"conversation_id"`
	Reply          string `json:"reply"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
}

// program is how the client names what it talks to when that has no name of
// its own: the command that starts it.
const program = "pn-scripts-assistant"

const (
	// defaultAPIURL is where the program serves unless told otherwise
	// (config.Default().Addr; a test keeps the two together).
	defaultAPIURL = "http://127.0.0.1:8790"

	// apiURLEnv is where to find it, under the program's name.
	apiURLEnv = "PN_SCRIPTS_ASSISTANT_API_URL"

	// legacyAPIURLEnv is the same from when the program was PN Brain. Still
	// read, so a script written for an older version keeps working.
	legacyAPIURLEnv = "BRAIN_API_URL"
)

// apiURLFromEnv is the address to talk to: the new variable, then the old one,
// then where the program serves by default.
func apiURLFromEnv() string {
	for _, key := range []string{apiURLEnv, legacyAPIURLEnv} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}

	return defaultAPIURL
}

type assistantInfo struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

/*
 * fetchInfo asks the program what it is called and whom it works for.
 *
 * /api/status is where the program says so. The client used to ask /api/brain,
 * which the program has never answered, so it always fell back to calling the
 * assistant "brain". /api/brain is still tried second, for anything older that
 * did answer there.
 */
func fetchInfo(apiURL string) (assistantInfo, error) {
	var last error

	for _, path := range []string{"/api/status", "/api/brain"} {
		info, err := getInfo(strings.TrimRight(apiURL, "/") + path)
		if err == nil {
			return info, nil
		}

		last = err
	}

	return assistantInfo{}, last
}

func getInfo(url string) (assistantInfo, error) {
	resp, err := http.Get(url)
	if err != nil {
		return assistantInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return assistantInfo{}, fmt.Errorf("%s answered %d", url, resp.StatusCode)
	}

	var info assistantInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return assistantInfo{}, err
	}

	if info.Name == "" && info.Owner == "" {
		return assistantInfo{}, errors.New(url + " did not say who it is")
	}

	return info, nil
}

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(dir, "pn-scripts-assistant")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(d, "cli.json"), nil
}

func loadState() cliState {
	p, err := statePath()
	if err != nil {
		return cliState{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		// Kept under the old name before the rename, which is still the
		// conversation this terminal was in.
		if b, err = os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(p)), "pn-brain", "cli.json")); err != nil {
			return cliState{}
		}
	}
	var s cliState
	_ = json.Unmarshal(b, &s)
	return s
}

func saveState(s cliState) {
	p, err := statePath()
	if err != nil {
		return
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
}

func send(apiURL string, s cliState, provider, message string) (chatResponse, error) {
	body := chatRequest{Message: message, ConversationID: s.ConversationID}
	if provider != "" {
		body.Provider = &provider
	}

	payload, _ := json.Marshal(body)
	resp, err := http.Post(strings.TrimRight(apiURL, "/")+"/api/chat", "application/json", bytes.NewReader(payload))
	if err != nil {
		return chatResponse{}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return chatResponse{}, fmt.Errorf("request failed (%d): %s", resp.StatusCode, string(raw))
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return chatResponse{}, err
	}
	return cr, nil
}

func runOne(apiURL string, s *cliState, provider, message, name string, stdout, stderr io.Writer) bool {
	cr, err := send(apiURL, *s, provider, message)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return false
	}
	s.ConversationID = &cr.ConversationID
	saveState(*s)
	fmt.Fprintf(stdout, "%s [%s/%s]> %s\n", strings.ToLower(name), cr.Provider, cr.Model, cr.Reply)
	return true
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole client, with its streams passed in so a test can read what
// a person would see.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pn-scripts-assistant-cli", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, "pn-scripts-assistant-cli talks to PN Scripts Assistant from a terminal.\n\n"+
			"  pn-scripts-assistant-cli [options] [message]\n\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nThe address can also be set with %s (%s is still read).\n",
			apiURLEnv, legacyAPIURLEnv)
	}

	apiURL := fs.String("api", apiURLFromEnv(), "address of PN Scripts Assistant on this machine")
	provider := fs.String("provider", "", "force provider: ollama or anthropic (default: server config)")
	newConv := fs.Bool("new", false, "start a new conversation instead of continuing the last one")
	version := fs.Bool("version", false, "say which version this is, and nothing else")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// Which copy this is, answered before anything reaches the network.
	if *version {
		fmt.Fprintln(stdout, release.Full("PN Scripts Assistant CLI"))
		return 0
	}

	s := loadState()
	if *newConv {
		s.ConversationID = nil
	}

	info, err := fetchInfo(*apiURL)
	name := program
	if err == nil && info.Name != "" {
		name = info.Name
	}

	if rest := fs.Args(); len(rest) > 0 {
		if !runOne(*apiURL, &s, *provider, strings.Join(rest, " "), name, stdout, stderr) {
			return 1
		}
		return 0
	}

	if err == nil {
		help := "How can I help?"
		if info.Owner != "" {
			help = "How can I help, " + info.Owner + "?"
		}
		fmt.Fprintf(stdout, "%s online. %s (':new' for a fresh conversation, 'exit' to quit)\n", name, help)
	} else {
		fmt.Fprintf(stdout, "Nothing answered at %s. Is PN Scripts Assistant running? Start it with %s, "+
			"or point this at it with --api. (':new' for a fresh conversation, 'exit' to quit)\n", *apiURL, program)
	}

	scanner := bufio.NewScanner(stdin)
	for {
		fmt.Fprint(stdout, "you> ")
		if !scanner.Scan() {
			fmt.Fprintln(stdout)
			break
		}
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == "exit" || line == "quit":
			return 0
		case line == ":new":
			s.ConversationID = nil
			fmt.Fprintln(stdout, "(new conversation)")
			continue
		}
		runOne(*apiURL, &s, *provider, line, name, stdout, stderr)
	}

	return 0
}
