// brain is a terminal client for the AI Brain API. It's the first of several
// clients that talk to the one brain (see ../../app/Services) over HTTP —
// each client picks whatever language fits its platform; only the brain
// itself is single-sourced.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(dir, "ai-brain")
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
		return cliState{}
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

func runOne(apiURL string, s *cliState, provider, message string) {
	cr, err := send(apiURL, *s, provider, message)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return
	}
	s.ConversationID = &cr.ConversationID
	saveState(*s)
	fmt.Printf("brain [%s/%s]> %s\n", cr.Provider, cr.Model, cr.Reply)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	apiURL := flag.String("api", envOr("BRAIN_API_URL", "http://localhost:8090"), "AI Brain API base URL")
	provider := flag.String("provider", "", "force provider: ollama or anthropic (default: server config)")
	newConv := flag.Bool("new", false, "start a new conversation instead of continuing the last one")
	flag.Parse()

	s := loadState()
	if *newConv {
		s.ConversationID = nil
	}

	if args := flag.Args(); len(args) > 0 {
		runOne(*apiURL, &s, *provider, strings.Join(args, " "))
		return
	}

	fmt.Println("AI Brain — interactive. ':new' for a fresh conversation, 'exit' or Ctrl+D to quit.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("you> ")
		if !scanner.Scan() {
			fmt.Println()
			break
		}
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == "exit" || line == "quit":
			return
		case line == ":new":
			s.ConversationID = nil
			fmt.Println("(new conversation)")
			continue
		}
		runOne(*apiURL, &s, *provider, line)
	}
}
