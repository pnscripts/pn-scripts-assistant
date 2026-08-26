// Package config holds the brain's settings.
//
// Settings come from a plain key=value file in the data root, overridden by the
// environment. The file lives with the data rather than with the program
// because it describes this brain — its name, its owner, its privacy setting —
// and that belongs to the disk that can be carried to another machine, not to
// the binary that happens to be running today.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is everything the brain needs to know about itself.
type Config struct {
	// Identity.
	Name  string
	Owner string

	// Privacy, as the raw configured string; parsing belongs to the llm package
	// so the strict-fallback rule lives in one place.
	Privacy string

	// Models.
	DefaultProvider string
	OllamaURL       string
	OllamaModel     string
	EmbedModel      string
	AnthropicKey    string
	AnthropicModel  string

	// Web search. Without a key the brain falls back to scraping DuckDuckGo,
	// which needs no account.
	BraveKey string

	// Smart home. Empty means no smart-home tools are offered at all.
	HomeAssistantURL   string
	HomeAssistantToken string

	// Serving.
	Addr string

	// Recall tuning. The defaults match the system this replaced, so answers do
	// not change character as a result of the port.
	RecallLimit int
	RecallFloor float64
}

// FileName is the settings file inside a data root.
const FileName = "brain.conf"

// Default returns settings that work on a machine with Ollama installed and
// nothing else configured.
//
// Privacy defaults to private. A default that leaks is a default that will be
// shipped by somebody who never read this file.
func Default() Config {
	return Config{
		Name:            "PN Brain",
		Owner:           "",
		Privacy:         "private",
		DefaultProvider: "ollama",
		OllamaURL:       "http://127.0.0.1:11434",
		OllamaModel:     "qwen2.5-coder:7b",
		EmbedModel:      "nomic-embed-text",
		AnthropicModel:  "",
		// Loopback only. Binding to every interface once exposed this brain's
		// knowledge endpoints to the local network, which is a mistake worth
		// making impossible rather than remembering not to make.
		Addr:        "127.0.0.1:8790",
		RecallLimit: 12,
		RecallFloor: 0.5,
	}
}

// Load reads the settings for a data root.
func Load(root string) (Config, error) {
	cfg := Default()

	values, err := readFile(filepath.Join(root, FileName))
	if err != nil {
		return cfg, err
	}

	// Environment wins over the file, so a one-off run can override without
	// editing anything.
	get := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}

		return values[key]
	}

	assign := func(dst *string, key string) {
		if v := get(key); v != "" {
			*dst = v
		}
	}

	assign(&cfg.Name, "BRAIN_NAME")
	assign(&cfg.Owner, "BRAIN_OWNER")
	assign(&cfg.Privacy, "BRAIN_PRIVACY")
	assign(&cfg.DefaultProvider, "LLM_DEFAULT_PROVIDER")
	assign(&cfg.OllamaURL, "OLLAMA_BASE_URL")
	assign(&cfg.OllamaModel, "OLLAMA_DEFAULT_MODEL")
	assign(&cfg.EmbedModel, "EMBEDDING_MODEL")
	assign(&cfg.AnthropicKey, "ANTHROPIC_API_KEY")
	assign(&cfg.AnthropicModel, "ANTHROPIC_MODEL")
	assign(&cfg.Addr, "BRAIN_ADDR")
	assign(&cfg.BraveKey, "BRAVE_SEARCH_KEY")
	assign(&cfg.HomeAssistantURL, "HOME_ASSISTANT_URL")
	assign(&cfg.HomeAssistantToken, "HOME_ASSISTANT_TOKEN")

	if v := get("RECALL_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RecallLimit = n
		}
	}

	if v := get("RECALL_FLOOR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RecallFloor = f
		}
	}

	return cfg, nil
}

// Save writes the settings a person is expected to edit.
//
// The API key is written here too, which is why the file is 0600: it is a
// credential sitting on a disk that is designed to be carried around.
func (c Config) Save(root string) error {
	path := filepath.Join(root, FileName)

	var b strings.Builder

	b.WriteString("# PN Brain settings. Environment variables override these.\n\n")
	b.WriteString("BRAIN_NAME=" + c.Name + "\n")
	b.WriteString("BRAIN_OWNER=" + c.Owner + "\n\n")
	b.WriteString("# private | research | open   (anything unrecognised is treated as private)\n")
	b.WriteString("BRAIN_PRIVACY=" + c.Privacy + "\n\n")
	b.WriteString("LLM_DEFAULT_PROVIDER=" + c.DefaultProvider + "\n")
	b.WriteString("OLLAMA_BASE_URL=" + c.OllamaURL + "\n")
	b.WriteString("OLLAMA_DEFAULT_MODEL=" + c.OllamaModel + "\n")
	b.WriteString("EMBEDDING_MODEL=" + c.EmbedModel + "\n")
	b.WriteString("ANTHROPIC_API_KEY=" + c.AnthropicKey + "\n")
	b.WriteString("ANTHROPIC_MODEL=" + c.AnthropicModel + "\n\n")
	b.WriteString("BRAIN_ADDR=" + c.Addr + "\n\n")
	b.WriteString("# Optional: a Brave Search API key. Without one, DuckDuckGo is scraped.\n")
	b.WriteString("BRAVE_SEARCH_KEY=" + c.BraveKey + "\n\n")
	b.WriteString("# Smart home. A long-lived access token from your Home Assistant profile.\n")
	b.WriteString("HOME_ASSISTANT_URL=" + c.HomeAssistantURL + "\n")
	b.WriteString("HOME_ASSISTANT_TOKEN=" + c.HomeAssistantToken + "\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// readFile parses key=value lines, ignoring blanks and # comments. A missing
// file is not an error: it means "all defaults", which is a valid state.
func readFile(path string) (map[string]string, error) {
	out := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}

		return out, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		value = strings.TrimSpace(value)

		// Tolerate quoted values, which people write out of habit.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		out[strings.TrimSpace(key)] = value
	}

	return out, scanner.Err()
}

// Path is where the settings file lives inside a data root.
//
// Exposed so callers that write settings — first-run setup, for one — do not
// have to reconstruct the path and drift from it.
func Path(root string) string { return filepath.Join(root, FileName) }
