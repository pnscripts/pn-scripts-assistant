// Package hostagent gives the brain reach beyond its container.
//
// PN Brain runs in Docker, which is what makes it installable and portable, and
// also what confines it: it sees three read-only mounts and nothing else. No
// wider filesystem, no LAN, no ability to run anything on the machine. That
// containment is a feature until the assistant is asked to do something real.
//
// This is the deliberate hole in that wall, and it is built to be a small one:
//
//   - It listens on loopback only. A daemon with shell access must never be
//     reachable from the network, and Docker's default port publishing already
//     put an unauthenticated brain on the LAN once in this project.
//   - Every request carries a shared token. Loopback is not an authorisation
//     boundary: any process on this machine, including a browser tab running
//     someone else's JavaScript, can reach 127.0.0.1.
//   - It refuses to read credentials, independently of the brain. The brain
//     has its own guard; this repeats it rather than trusting a caller that
//     might one day be a different program, or a compromised one.
//
// What it does not do is decide anything. Whether an action is allowed to
// happen at all is settled before a request reaches here, by the permission
// gate in the brain. This end just carries it out and reports honestly.
package hostagent

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxReadBytes caps a file read so one large file cannot exhaust memory here or
// the model's context on the other side.
const maxReadBytes = 200_000

// commandTimeout stops a hung command holding the agent open forever.
const commandTimeout = 60 * time.Second

type Agent struct {
	token string
}

func New(token string) *Agent {
	return &Agent{token: token}
}

// Handler builds the routes. Kept separate from serving so tests can drive it
// without binding a port.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "host": hostname()})
	})

	mux.HandleFunc("/read", a.authed(a.handleRead))
	mux.HandleFunc("/list", a.authed(a.handleList))
	mux.HandleFunc("/exec", a.authed(a.handleExec))

	return mux
}

// authed rejects anything without the shared token, in constant time.
//
// Comparing with == would leak the token a character at a time to anything that
// can measure response latency, which on loopback is everything.
func (a *Agent) authed(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		if subtle.ConstantTimeCompare([]byte(supplied), []byte(a.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "bad token"})

			return
		}

		next(w, r)
	}
}

type pathRequest struct {
	Path string `json:"path"`
}

func (a *Agent) handleRead(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if !decode(w, r, &req) {
		return
	}

	if err := guardSensitive(req.Path); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})

		return
	}

	info, err := os.Stat(req.Path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})

		return
	}

	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "that is a directory; use /list"})

		return
	}

	data, err := os.ReadFile(req.Path)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})

		return
	}

	truncated := false
	if len(data) > maxReadBytes {
		data = data[:maxReadBytes]
		truncated = true
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"content":   string(data),
		"truncated": truncated,
		"bytes":     info.Size(),
	})
}

func (a *Agent) handleList(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if !decode(w, r, &req) {
		return
	}

	entries, err := os.ReadDir(req.Path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})

		return
	}

	names := make([]string, 0, len(entries))

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}

		names = append(names, name)
	}

	sort.Strings(names)

	writeJSON(w, http.StatusOK, map[string]any{"entries": names})
}

type execRequest struct {
	Command string `json:"command"`
	Dir     string `json:"dir"`
}

// handleExec runs a command on the host.
//
// It takes an argv array rather than a shell string, so there is no shell to
// inject into: no globbing, no pipelines, no `; rm -rf`. A model that wants a
// pipeline has to be told to ask for one explicitly, which is the point — the
// approval a human gave was for a command they could read.
func (a *Agent) handleExec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Argv []string `json:"argv"`
		Dir  string   `json:"dir"`
	}

	if !decode(w, r, &req) {
		return
	}

	if len(req.Argv) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no command given"})

		return
	}

	cmd := exec.Command(req.Argv[0], req.Argv[1:]...)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}

	done := make(chan struct{})

	var output []byte
	var runErr error

	go func() {
		output, runErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(commandTimeout):
		_ = cmd.Process.Kill()
		writeJSON(w, http.StatusOK, map[string]any{
			"output":    string(output),
			"exit_code": -1,
			"error":     fmt.Sprintf("command exceeded %s and was stopped", commandTimeout),
		})

		return
	}

	exitCode := 0

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if runErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"output":    string(output),
			"exit_code": -1,
			"error":     runErr.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"output":    string(output),
		"exit_code": exitCode,
	})
}

// guardSensitive repeats the brain's refusal to touch credentials.
//
// Duplicated on purpose. The brain checks before calling, but this daemon has
// shell access to a whole machine and should not depend on every future caller
// being careful — or being the brain at all.
func guardSensitive(path string) error {
	name := strings.ToLower(filepath.Base(path))
	lower := strings.ToLower(filepath.ToSlash(path))

	if strings.HasPrefix(name, ".env") {
		return errors.New("refusing to read files holding credentials")
	}

	for _, exact := range []string{
		".npmrc", ".netrc", ".git-credentials", ".pgpass", ".my.cnf",
		"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa", "credentials", "auth.json", "shadow",
	} {
		if name == exact {
			return errors.New("refusing to read files holding credentials")
		}
	}

	for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".keystore", ".jks"} {
		if strings.HasSuffix(name, ext) {
			return errors.New("refusing to read files holding credentials")
		}
	}

	for _, dir := range []string{
		"/.ssh/", "/.gnupg/", "/.aws/", "/.azure/", "/.kube/",
		"/.docker/", "/.password-store/", "/etc/shadow", "/.mozilla/", "/.thunderbird/",
	} {
		if strings.Contains(lower, dir) {
			return errors.New("refusing to read files holding credentials")
		}
	}

	return nil
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed request"})

		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}

	return name
}
