package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
)

/*
 * A server that is a program on this machine, spoken to on its standard input
 * and output, one JSON message a line.
 *
 * Started from an argv — never a shell line — with an environment built for
 * it rather than inherited: the path, a home, a language, and the secrets its
 * owner gave this one server, and nothing else of this program's. Its own
 * chatter on stderr is kept, the last lines of it, for saying why it died.
 */
type Stdio struct {
	cmd *exec.Cmd
	in  io.WriteCloser

	writing sync.Mutex

	waiting sync.Mutex
	pending map[int64]chan message

	said *lines

	done   chan struct{}
	failed error
}

// StartStdio starts a server program.
func StartStdio(argv []string, env []string, dir string) (*Stdio, error) {
	if len(argv) == 0 {
		return nil, errors.New("no program to run")
	}

	/*
	 * A server is a program somebody else wrote, running as its owner. Held
	 * by the kernel to writing in its own folder and the caches its runtime
	 * keeps — npm's and uv's — so a server approved to read a calendar cannot
	 * also rewrite a shell profile. In a process group of its own, so
	 * closing it closes whatever it started.
	 */
	home, _ := os.UserHomeDir()

	writable := []string{dir, os.TempDir(), "/dev"}
	if home != "" {
		writable = append(writable, filepath.Join(home, ".npm"), filepath.Join(home, ".cache"),
			filepath.Join(home, ".local", "share", "uv"))
	}

	cmd, err := sandbox.Command(context.Background(), sandbox.Spec{Dir: dir, Env: env, Writable: writable}, argv...)
	if err != nil {
		return nil, err
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	s := &Stdio{cmd: cmd, in: in, pending: map[int64]chan message{}, said: &lines{most: 40}, done: make(chan struct{})}

	cmd.Stderr = s.said

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start %s: %w", argv[0], err)
	}

	go s.read(out)

	return s, nil
}

// read hands each answer to whoever is waiting for it, and answers the
// server's own requests with "not here" — this program offers servers no
// sampling, no roots and no elicitation.
func (s *Stdio) read(out io.Reader) {
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)

	for scanner.Scan() {
		var m message

		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			continue
		}

		switch {
		case m.ID != nil && m.Method == "":
			s.waiting.Lock()
			ch := s.pending[*m.ID]
			delete(s.pending, *m.ID)
			s.waiting.Unlock()

			if ch != nil {
				ch <- m
			}

		case m.ID != nil && m.Method != "":
			reply := message{JSONRPC: "2.0", ID: m.ID}

			if m.Method == "ping" {
				reply.Result = json.RawMessage(`{}`)
			} else {
				reply.Error = &RPCError{Code: codeMethodNotFound, Message: "this client does not offer " + m.Method}
			}

			s.write(reply)
		}
	}

	s.failed = s.cmd.Wait()
	close(s.done)

	s.waiting.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.waiting.Unlock()
}

func (s *Stdio) write(m message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}

	s.writing.Lock()
	defer s.writing.Unlock()

	_, err = s.in.Write(append(raw, '\n'))

	return err
}

// Call sends a request and waits for its answer, the server's death, or the
// deadline — whichever is first.
func (s *Stdio) Call(ctx context.Context, req message, _, _ string) (message, error) {
	ch := make(chan message, 1)

	s.waiting.Lock()
	s.pending[*req.ID] = ch
	s.waiting.Unlock()

	if err := s.write(req); err != nil {
		/*
		 * Its input closed because it has already gone: what it said on the
		 * way out is the answer, not the broken pipe. A server that exits at
		 * once — a missing runtime, a package that would not install — says
		 * why on stderr, and that is what somebody needs to read.
		 */
		select {
		case <-s.done:
			return message{}, s.died()
		case <-time.After(2 * time.Second):
		}

		return message{}, fmt.Errorf("the server is not listening: %w", err)
	}

	select {
	case m, ok := <-ch:
		if !ok {
			return message{}, s.died()
		}

		return m, nil
	case <-s.done:
		return message{}, s.died()
	case <-ctx.Done():
		s.waiting.Lock()
		delete(s.pending, *req.ID)
		s.waiting.Unlock()

		return message{}, fmt.Errorf("the server did not answer %s in time", req.Method)
	}
}

func (s *Stdio) died() error {
	why := "it stopped"
	if s.failed != nil {
		why = s.failed.Error()
	}

	if said := s.said.last(6); said != "" {
		why += " — it said: " + said
	}

	return errors.New("the server is not running: " + why)
}

// Notify sends a notification.
func (s *Stdio) Notify(_ context.Context, m message, _ string) error { return s.write(m) }

// Close asks the server to stop by closing its input, then makes it.
func (s *Stdio) Close() error {
	s.in.Close()

	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		<-s.done
	}

	return nil
}

// Said is the last things the server wrote on stderr.
func (s *Stdio) Said() string { return s.said.last(20) }

// Running is whether the program is still there.
func (s *Stdio) Running() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// lines keeps the last lines written to it.
type lines struct {
	mu   sync.Mutex
	most int
	kept []string
	part string
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	text := l.part + string(p)
	parts := strings.Split(text, "\n")
	l.part = parts[len(parts)-1]

	for _, line := range parts[:len(parts)-1] {
		if line = strings.TrimSpace(line); line != "" {
			l.kept = append(l.kept, line)
		}
	}

	if len(l.kept) > l.most {
		l.kept = l.kept[len(l.kept)-l.most:]
	}

	return len(p), nil
}

func (l *lines) last(n int) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	kept := l.kept
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}

	return strings.Join(kept, " | ")
}
