package preflight

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

/*
 * Asking Ollama whether it is there, rather than looking for its command.
 *
 * Every one of these checks used to shell out to `ollama`, and a machine
 * without that command on its PATH was told Ollama was missing and offered a
 * 1.5 GB download — while a perfectly good Ollama answered on the port the
 * program talks to. Three ordinary ways to end up in that state: it is
 * installed as a system service and the CLI is not on this user's PATH, it
 * lives in a home directory the program cannot see, or OLLAMA_BASE_URL points
 * at another machine entirely.
 *
 * The program only ever talks to Ollama over HTTP. So that is the question:
 * is something answering, and does it have the models. The command still
 * matters for installing one — `ollama pull` — and that is the only thing it
 * is asked for now.
 */

// HowLongToAsk bounds the question. Local and already running, or not.
const HowLongToAsk = 2 * time.Second

// OllamaURL is where Ollama should be answering.
func OllamaURL() string {
	if set := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")); set != "" {
		return strings.TrimSuffix(set, "/")
	}

	return "http://127.0.0.1:11434"
}

// ollamaAnswering reports whether something is there to talk to.
func ollamaAnswering() bool {
	_, ok := ollamaModels()

	return ok
}

// ollamaModels is what it has, and whether it answered at all. The two are
// different: no models is a machine that needs one, and no answer is a
// machine that needs Ollama.
func ollamaModels() ([]string, bool) {
	client := &http.Client{Timeout: HowLongToAsk}

	resp, err := client.Get(OllamaURL() + "/api/tags")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false
	}

	names := make([]string, 0, len(body.Models))

	for _, model := range body.Models {
		if model.Name != "" {
			names = append(names, model.Name)
		}
	}

	return names, true
}

// ollamaHere reports whether Ollama can be used at all: the command, or
// something answering on the address.
func ollamaHere() bool { return commandExists("ollama") || ollamaAnswering() }

/*
 * The Ollama already on this machine, and how new it is.
 *
 * Asked before installing, so that one somebody keeps current themselves is
 * not replaced by the older release pinned here. Three places it can be: the
 * one answering on the address (a system service, another machine), the
 * command on the PATH, and the one this program puts in ~/.local/bin. The
 * newest of those is what counts; an empty version means none of them could
 * say.
 */
type ollamaFound struct {
	version   string
	where     string
	binary    string
	answering bool
}

func ollamaAtHand() ollamaFound {
	var best ollamaFound

	consider := func(found ollamaFound) {
		if _, ok := versionNumbers(found.version); !ok {
			return
		}

		if best.version == "" || IsNewer(found.version, best.version) {
			best = found
		}
	}

	if v := ollamaServerVersion(); v != "" {
		consider(ollamaFound{version: v, where: "answering at " + OllamaURL(), answering: true})
	}

	seen := map[string]bool{}

	for _, binary := range []string{whereIs("ollama"), ourOllama()} {
		if binary == "" || seen[binary] {
			continue
		}

		seen[binary] = true

		if v := ollamaBinaryVersion(binary); v != "" {
			consider(ollamaFound{version: v, where: binary, binary: binary})
		}
	}

	// One that answers needs nothing started, whichever program it is.
	if best.answering {
		best.binary = ""
	}

	return best
}

// ourOllama is where installOllama puts the program.
func ourOllama() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "bin", "ollama")
}

// ollamaServerVersion is what the Ollama on the address says it is.
func ollamaServerVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), HowLongToAsk)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OllamaURL()+"/api/version", nil)
	if err != nil {
		return ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var body struct {
		Version string `json:"version"`
	}

	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil {
		return ""
	}

	return strings.TrimSpace(body.Version)
}

/*
 * ollamaBinaryVersion asks a program which version it is.
 *
 * "ollama version is 0.40.11" with a server of the same version running;
 * with none, a warning first and then "client version is 0.40.11"; with a
 * server of another version, the server's first and the program's as "client
 * version is". So the client's line wins, and any "version is" otherwise.
 */
func ollamaBinaryVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, _ := exec.CommandContext(ctx, binary, "--version").CombinedOutput()

	// With a server of another version running, the first line is the
	// server's and the program's own comes after it as "client version is".
	var said string

	for _, line := range strings.Split(string(out), "\n") {
		at := strings.Index(line, "version is ")
		if at < 0 {
			continue
		}

		version := strings.TrimSpace(line[at+len("version is "):])

		if strings.Contains(line, "client version is ") {
			return version
		}

		if said == "" {
			said = version
		}
	}

	return said
}

// ollamaFetch and ollamaStart are how installOllama downloads and starts the
// service; a test puts recorders here, so that nothing is fetched from the
// internet and the user's own services are never touched.
var (
	ollamaFetch = Fetch
	ollamaStart = startOllama
)
