package vouch

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTheKeyIsMadeOnceAndOnlyItsAccountReadsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "pn-scripts-assistant", FileName)

	if _, err := Read(path); !errors.Is(err, ErrNoKey) {
		t.Fatalf("a key was read before any was written: %v", err)
	}

	key, err := Keep(path)
	if err != nil || len(key) != keyBytes {
		t.Fatalf("keeping a key: %d bytes, %v", len(key), err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("the key's file is %o, not 600", info.Mode().Perm())
	}

	raw, _ := os.ReadFile(path)
	if strings.TrimSpace(string(raw)) != hex.EncodeToString(key) {
		t.Errorf("the file does not hold the key as hex")
	}

	// The next start, and whoever asks, read the same one.
	again, err := Keep(path)
	if err != nil || !bytes.Equal(again, key) {
		t.Errorf("the second start made another key: %v", err)
	}

	read, err := Read(path)
	if err != nil || !bytes.Equal(read, key) {
		t.Errorf("whoever asks read another key: %v", err)
	}

	other, _ := Keep(filepath.Join(t.TempDir(), FileName))
	if bytes.Equal(other, key) {
		t.Error("two computers were given the same key")
	}
}

/*
 * A key other accounts can read, a link, a folder, half a key or anything
 * that is not one: whoever asks believes none of them, and the assistant
 * replaces them when it starts.
 */
func TestAKeyThatCannotBeTrustedIsNotReadAndIsReplaced(t *testing.T) {
	good := hex.EncodeToString(bytes.Repeat([]byte{7}, keyBytes))

	spoil := map[string]func(t *testing.T, path string){
		"half a key":  func(t *testing.T, path string) { os.WriteFile(path, []byte(good[:keyBytes]), 0o600) },
		"not hex":     func(t *testing.T, path string) { os.WriteFile(path, []byte(strings.Repeat("z", 2*keyBytes)), 0o600) },
		"empty":       func(t *testing.T, path string) { os.WriteFile(path, nil, 0o600) },
		"far too big": func(t *testing.T, path string) { os.WriteFile(path, []byte(strings.Repeat(good, 40)), 0o600) },
	}

	if runtime.GOOS != "windows" {
		spoil["readable by the group"] = func(t *testing.T, path string) {
			os.WriteFile(path, []byte(good), 0o600)
			os.Chmod(path, 0o640)
		}
		spoil["writable by everybody"] = func(t *testing.T, path string) {
			os.WriteFile(path, []byte(good), 0o600)
			os.Chmod(path, 0o602)
		}
		spoil["a link to a key"] = func(t *testing.T, path string) {
			real := filepath.Join(filepath.Dir(path), "elsewhere")
			os.WriteFile(real, []byte(good), 0o600)

			if err := os.Symlink(real, path); err != nil {
				t.Skip("no links here")
			}
		}
	}

	for name, how := range spoil {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			how(t, path)

			if key, err := Read(path); !errors.Is(err, ErrNoKey) || key != nil {
				t.Fatalf("%s was read as a key: %v", name, err)
			}

			key, err := Keep(path)
			if err != nil || len(key) != keyBytes || hex.EncodeToString(key) == good {
				t.Fatalf("%s was not replaced by a new key: %v", name, err)
			}

			if read, err := Read(path); err != nil || !bytes.Equal(read, key) {
				t.Errorf("after replacing %s the key is not read: %v", name, err)
			}
		})
	}

	// A folder where the file belongs is not removed with what is in it.
	path := filepath.Join(t.TempDir(), FileName)
	os.MkdirAll(filepath.Join(path, "inside"), 0o700)

	if _, err := Read(path); !errors.Is(err, ErrNoKey) {
		t.Error("a folder was read as a key")
	}

	if _, err := Keep(path); err == nil {
		t.Error("a folder full of something was replaced by a key")
	}
}

func TestStartsAtTheSameMomentAgreeOnOneKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		keys = map[string]bool{}
	)

	for i := 0; i < 24; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			key, err := Keep(path)
			if err != nil {
				t.Errorf("keeping: %v", err)

				return
			}

			mu.Lock()
			keys[hex.EncodeToString(key)] = true
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(keys) != 1 {
		t.Errorf("%d different keys from starts at the same moment", len(keys))
	}
}

// Every part under the seal matters: change any one and it no longer holds.
func TestTheSealHoldsOnlyForExactlyWhatWasAskedAndAnswered(t *testing.T) {
	key := bytes.Repeat([]byte{1}, keyBytes)
	number, err := Number()

	if err != nil || !numberLike(number) {
		t.Fatalf("a number: %q %v", number, err)
	}

	if again, _ := Number(); again == number {
		t.Fatal("two requests were given the same number")
	}

	asked, answer := []byte(`{"tool":"permission_prompt"}`), []byte(`{"behavior":"allow"}`)
	seal := Seal(key, number, "POST", "/api/outside/ask", asked, 200, answer)

	if !Holds(key, number, "POST", "/api/outside/ask", asked, 200, answer, seal) {
		t.Fatal("the seal does not hold for what it was made of")
	}

	otherNumber, _ := Number()

	for name, holds := range map[string]bool{
		"another key":     Holds(bytes.Repeat([]byte{2}, keyBytes), number, "POST", "/api/outside/ask", asked, 200, answer, seal),
		"no key":          Holds(nil, number, "POST", "/api/outside/ask", asked, 200, answer, seal),
		"a short key":     Holds(key[:8], number, "POST", "/api/outside/ask", asked, 200, answer, Seal(key[:8], number, "POST", "/api/outside/ask", asked, 200, answer)),
		"another number":  Holds(key, otherNumber, "POST", "/api/outside/ask", asked, 200, answer, seal),
		"no number":       Holds(key, "", "POST", "/api/outside/ask", asked, 200, answer, Seal(key, "", "POST", "/api/outside/ask", asked, 200, answer)),
		"another method":  Holds(key, number, "GET", "/api/outside/ask", asked, 200, answer, seal),
		"another path":    Holds(key, number, "POST", "/api/outside/allow", asked, 200, answer, seal),
		"another request": Holds(key, number, "POST", "/api/outside/ask", []byte(`{"tool":"assistant_status"}`), 200, answer, seal),
		"another status":  Holds(key, number, "POST", "/api/outside/ask", asked, 403, answer, seal),
		"another answer":  Holds(key, number, "POST", "/api/outside/ask", asked, 200, []byte(`{"behavior":"deny"}`), seal),
		"no seal":         Holds(key, number, "POST", "/api/outside/ask", asked, 200, answer, ""),
		"a seal cut":      Holds(key, number, "POST", "/api/outside/ask", asked, 200, answer, seal[:len(seal)-1]),
		// Parts that run into each other when simply joined.
		"path and method moved": Holds(key, number, "POS", "T/api/outside/ask", asked, 200, answer, seal),
	} {
		if holds {
			t.Errorf("the seal held with %s", name)
		}
	}
}

// sealed asks a sealing handler one thing and reads all of the answer.
func sealed(t *testing.T, url, method, body, number string) (*http.Response, []byte) {
	t.Helper()

	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if number != "" {
		req.Header.Set(AskHeader, number)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	return res, raw
}

func TestAnAnswerIsSealedOnlyWhenAskedAndAsItWasGiven(t *testing.T) {
	key := bytes.Repeat([]byte{3}, keyBytes)

	var got string

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = string(raw)

		// A handler that moves the deadline, as the door does for a
		// permission question, must reach the connection through this.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute)); err != nil {
			t.Errorf("the deadline could not be moved through the seal: %v", err)
		}

		switch r.URL.Path {
		case "/refuse":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.WriteHeader(http.StatusOK) // a second word is not heard
			fmt.Fprint(w, `{"error":"no"}`)
		case "/nothing":
			w.WriteHeader(http.StatusNoContent)
		case "/huge":
			// More than is ever held: written in pieces, as a download is.
			piece := bytes.Repeat([]byte{'x'}, 1<<20)

			for i := 0; i <= MostAnswered>>20; i++ {
				if _, err := w.Write(piece); err != nil {
					return
				}
			}

			t.Error("an answer over the size was taken whole")
		default:
			fmt.Fprint(w, `{"part":1,`)
			fmt.Fprint(w, `"part2":2}`)
		}
	})

	theirs := true

	ts := httptest.NewServer(Sealing(func(*http.Request) []byte {
		if theirs {
			return key
		}

		return nil
	}, inner))
	defer ts.Close()

	number, _ := Number()

	// Asked: the body reaches the handler whole, the answer comes whole
	// and the seal is of exactly the two.
	res, raw := sealed(t, ts.URL+"/ask", "POST", `{"q":1}`, number)
	if got != `{"q":1}` || string(raw) != `{"part":1,"part2":2}` || res.StatusCode != 200 {
		t.Fatalf("through the seal: handler read %q, answer %d %q", got, res.StatusCode, raw)
	}

	if !Holds(key, number, "POST", "/ask", []byte(`{"q":1}`), 200, raw, res.Header.Get(SealHeader)) {
		t.Error("the answer's seal does not hold")
	}

	// A refusal and an answer with nothing in it are sealed like the rest.
	res, raw = sealed(t, ts.URL+"/refuse", "GET", "", number)
	if res.StatusCode != http.StatusForbidden || !Holds(key, number, "GET", "/refuse", nil, 403, raw, res.Header.Get(SealHeader)) ||
		res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("a refusal: %d %q, sealed %v", res.StatusCode, raw, res.Header.Get(SealHeader) != "")
	}

	res, raw = sealed(t, ts.URL+"/nothing", "GET", "", number)
	if res.StatusCode != http.StatusNoContent || !Holds(key, number, "GET", "/nothing", nil, 204, raw, res.Header.Get(SealHeader)) {
		t.Errorf("an answer with nothing in it: %d, sealed %v", res.StatusCode, res.Header.Get(SealHeader) != "")
	}

	// Not asked, or asked with something that is not a number: as ever,
	// and no seal.
	for _, n := range []string{"", "1234", strings.Repeat("G", 2*numberBytes), strings.Repeat("a", 2*numberBytes+2)} {
		res, raw = sealed(t, ts.URL+"/ask", "POST", `{}`, n)
		if res.Header.Get(SealHeader) != "" || string(raw) != `{"part":1,"part2":2}` {
			t.Errorf("number %q: seal %q, answer %q", n, res.Header.Get(SealHeader), raw)
		}
	}

	// Somebody the key is not for (a device elsewhere): no seal.
	theirs = false

	if res, _ = sealed(t, ts.URL+"/ask", "POST", `{}`, number); res.Header.Get(SealHeader) != "" {
		t.Error("an answer was sealed for somebody the key is not for")
	}

	theirs = true

	// What follows the path is under the seal too.
	res, raw = sealed(t, ts.URL+"/ask?kind=deb", "POST", `{}`, number)
	if !Holds(key, number, "POST", "/ask?kind=deb", []byte(`{}`), 200, raw, res.Header.Get(SealHeader)) ||
		Holds(key, number, "POST", "/ask", []byte(`{}`), 200, raw, res.Header.Get(SealHeader)) ||
		Holds(key, number, "POST", "/ask?kind=snap", []byte(`{}`), 200, raw, res.Header.Get(SealHeader)) {
		t.Error("the seal does not cover what follows the path")
	}

	// An answer too large to hold is not sent, in part or whole.
	res, raw = sealed(t, ts.URL+"/huge", "GET", "", number)
	if res.StatusCode != http.StatusInternalServerError || res.Header.Get(SealHeader) != "" || len(raw) > 1024 {
		t.Errorf("an answer over the size: %d, %d bytes, sealed %v", res.StatusCode, len(raw), res.Header.Get(SealHeader) != "")
	}

	// A request too large to hold is refused, not sealed in part.
	res, _ = sealed(t, ts.URL+"/ask", "POST", strings.Repeat("x", MostAsked+1), number)
	if res.StatusCode != http.StatusRequestEntityTooLarge || res.Header.Get(SealHeader) != "" {
		t.Errorf("a request over the size: %d, sealed %v", res.StatusCode, res.Header.Get(SealHeader) != "")
	}
}

func TestThePlaceIsInTheAccountsSettingsFolder(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the settings folder is named by XDG_CONFIG_HOME on Linux")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	place, err := Place()
	if err != nil || place != filepath.Join(dir, "pn-scripts-assistant", FileName) {
		t.Errorf("the key's place: %q %v", place, err)
	}
}

// A file system with no links (some network and removable ones): the key
// is still made once, for this account only, and read again.
func TestTheKeyIsKeptWhereThereAreNoLinks(t *testing.T) {
	before := link
	link = func(string, string) error { return errors.New("operation not supported") }

	defer func() { link = before }()

	path := filepath.Join(t.TempDir(), FileName)

	key, err := Keep(path)
	if err != nil || len(key) != keyBytes {
		t.Fatalf("keeping a key with no links: %v", err)
	}

	if again, err := Keep(path); err != nil || !bytes.Equal(again, key) {
		t.Errorf("the second start made another key: %v", err)
	}

	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("the key's file is %o, not 600", info.Mode().Perm())
	}

	left, _ := os.ReadDir(filepath.Dir(path))
	if len(left) != 1 {
		t.Errorf("%d files were left beside the key", len(left)-1)
	}
}

/*
 * Clearing away what is not a key never takes a key away. Between looking
 * and clearing, another start may have put a good key there: it is put
 * back, and the copy that made it keeps sealing with what is on disk.
 */
func TestAGoodKeyIsNeverClearedAway(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	key, err := Keep(path)
	if err != nil {
		t.Fatal(err)
	}

	// As the start that lost the race does it: it saw something that was
	// not a key, and by now the other start's key is there.
	if err := setAside(path); err != nil {
		t.Fatal(err)
	}

	if read, err := Read(path); err != nil || !bytes.Equal(read, key) {
		t.Fatalf("a good key was cleared away: %v", err)
	}

	// With nothing there it does nothing, and leaves nothing behind.
	gone := filepath.Join(t.TempDir(), FileName)

	if err := setAside(gone); err != nil {
		t.Errorf("clearing away nothing: %v", err)
	}

	if left, _ := os.ReadDir(filepath.Dir(gone)); len(left) != 0 {
		t.Errorf("clearing away nothing left %d files", len(left))
	}

	if left, _ := os.ReadDir(filepath.Dir(path)); len(left) != 1 {
		t.Errorf("%d files were left beside the key", len(left)-1)
	}
}

// A file this account may not read (left by a start under another
// account) is never this account's key: it is replaced, not waited for.
func TestAKeyThisAccountCannotReadIsReplaced(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not keep this account out here")
	}

	path := filepath.Join(t.TempDir(), FileName)

	if err := os.WriteFile(path, []byte(strings.Repeat("ab", keyBytes)), 0o000); err != nil {
		t.Fatal(err)
	}

	if _, err := Read(path); !errors.Is(err, ErrNoKey) {
		t.Fatal("a file that cannot be read was read as a key")
	}

	key, err := Keep(path)
	if err != nil || len(key) != keyBytes {
		t.Fatalf("a key this account cannot read was not replaced: %v", err)
	}

	if read, err := Read(path); err != nil || !bytes.Equal(read, key) {
		t.Errorf("after replacing it the key is not read: %v", err)
	}
}
