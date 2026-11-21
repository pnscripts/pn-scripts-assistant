/*
 * Package vouch is how an answer on the assistant's address is shown to be
 * the assistant's own.
 *
 * "pn-scripts-assistant mcp" asks the running assistant over its address
 * on this computer and passes on what it is told; a permission question of
 * Claude Code is answered "allow" or "deny" that way. An address is not a
 * proof of who answers on it. When the assistant is not running its port is
 * free, any program may listen there, and whatever it answered was taken as
 * the assistant's word: "allow" to a permission question nobody was asked
 * (a note of the review of 1.2.0).
 *
 * So the two share a key that never travels. The assistant keeps it in a
 * file only its user's account can read, in that account's settings folder
 * on this computer (not in the brain: a brain may live on a stick whose
 * file system has no permissions, and the key is about this computer's
 * address, not about what the assistant knows). Whoever asks sends a number
 * made for that one request; the answer comes back with a seal over that
 * number, the request as it was sent and the answer as it was given
 * (HMAC-SHA-256). The asker makes the same seal from the file and passes
 * nothing on that does not carry it.
 *
 * What is under the seal, and why each part:
 *   - the number: an answer kept from another request does not fit;
 *   - the method, the path and the request's own body: a seal obtained for
 *     one question does not fit another. Without the body, a program
 *     between the two could ask the real assistant an innocent permission
 *     question under the same number, have its user say yes to that, and
 *     hand the yes to the dangerous question it was holding;
 *   - the status and the answer's body: nothing of the answer can be
 *     changed on the way.
 *
 * What this does not do, because the limit is real: a program running
 * under the same account can read the file, as it can read everything else
 * of that account, and can then answer as the assistant. It stops a program
 * that can listen on a port but cannot read the account's files (another
 * account on the computer, a confined program), and it stops an answer
 * being changed or replayed by something between the two.
 */
package vouch

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	// AskHeader carries the number of one request, from whoever asks.
	AskHeader = "X-PN-Prove"

	// SealHeader carries the seal on the answer.
	SealHeader = "X-PN-Proof"

	// FileName is the key's file in the account's settings folder for
	// this program.
	FileName = "answers.key"

	// folder is this program's folder there (paths.Name; a test in
	// package server keeps the two together).
	folder = "pn-scripts-assistant"

	// keyBytes is the key's size; in the file it is hex, on one line.
	keyBytes = 32

	// numberBytes is the size of a request's number.
	numberBytes = 16

	// MostAsked is the largest request whose answer is sealed. What other
	// programs ask is a few kilobytes at most.
	MostAsked = 1 << 20

	// MostAnswered is the largest answer that is held to be sealed: the
	// longest list of programs is far under it.
	MostAnswered = 32 << 20
)

// ErrNoKey is no usable key on this computer: none was written yet, or
// what is there is not a key or is open to other accounts.
var ErrNoKey = errors.New("this computer holds no key to tell PN Scripts Assistant's answers by")

// Place is where the key is kept for the account this runs under.
func Place() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, folder, FileName), nil
}

// Here reads the key of the account this runs under, from its Place.
func Here() ([]byte, error) {
	place, err := Place()
	if err != nil {
		return nil, ErrNoKey
	}

	return Read(place)
}

/*
 * Read is the key as whoever asks reads it: only a plain file, only one
 * that no other account can read or write, only a whole key. Anything else
 * is ErrNoKey, and with no key nothing is believed.
 */
func Read(path string) ([]byte, error) {
	key, state := look(path)
	if state != whole {
		return nil, ErrNoKey
	}

	return key, nil
}

// What look found at the key's place.
const (
	whole    = iota // a key nobody else can touch
	absent          // nothing there
	notAKey         // something there that is not such a key
	unlooked        // it could not be looked at just now
)

/*
 * look reads what is at path and says which of the four it is.
 *
 * The file is looked at before it is opened (a link, a folder or a pipe
 * is never opened) and again through the open handle, which must be the
 * same file: what was checked is what is read, and no more of it than a
 * key is long.
 */
func look(path string) ([]byte, int) {
	before, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, absent
	}

	if err != nil {
		return nil, unlooked
	}

	if !before.Mode().IsRegular() {
		return nil, notAKey
	}

	f, err := os.Open(path)

	switch {
	case os.IsNotExist(err):
		return nil, absent
	case os.IsPermission(err):
		// A plain file in this account's own folder that this account
		// may not read (left by a start under another account) will
		// never be this account's key.
		return nil, notAKey
	case err != nil:
		return nil, unlooked
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, unlooked
	}

	if !os.SameFile(before, info) || !info.Mode().IsRegular() || (private() && info.Mode().Perm()&0o077 != 0) {
		return nil, notAKey
	}

	raw, err := io.ReadAll(io.LimitReader(f, 4*keyBytes))
	if err != nil {
		return nil, unlooked
	}

	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != keyBytes {
		return nil, notAKey
	}

	return key, whole
}

// private is whether file modes say who may read a file here. On Windows
// they do not; the settings folder there is the account's own.
func private() bool { return runtime.GOOS != "windows" }

/*
 * Keep is the key as the assistant takes it when it starts: the one that
 * is there, or a new one when there is none or what is there is not a key
 * that nobody else can touch (see Read).
 *
 * Kept from one start to the next, so that several copies of the program
 * under one account (a second one on another port) seal with the same key
 * and neither undoes the other's. For the same reason a key is replaced
 * only when what is there is certainly not one: a file that could not be
 * looked at this moment is left, and the start goes on without a key
 * rather than take a running copy's key from under it.
 *
 * A new key is written whole beside its place and then linked into it,
 * which fails when something is there already: the file is never there
 * half written, and of two starts at the same moment one makes it and the
 * other reads what the first made (place, setAside).
 */
func Keep(path string) ([]byte, error) {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("the folder for the key of the assistant's answers: %w", err)
	}

	for try := 0; try < 4; try++ {
		key, state := look(path)

		switch state {
		case whole:
			return key, nil
		case unlooked:
			return nil, errors.New("the key of the assistant's answers could not be read just now, and was left as it is")
		case notAKey:
			if err := setAside(path); err != nil {
				return nil, fmt.Errorf("replacing the key of the assistant's answers: %w", err)
			}
		}

		key = make([]byte, keyBytes)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("no randomness for the key of the assistant's answers: %w", err)
		}

		made, err := os.CreateTemp(dir, FileName+".new-*")
		if err != nil {
			return nil, fmt.Errorf("writing the key of the assistant's answers: %w", err)
		}

		// CreateTemp makes it for this account only (0600).
		_, err = made.WriteString(hex.EncodeToString(key) + "\n")
		if closed := made.Close(); err == nil {
			err = closed
		}

		if err != nil {
			os.Remove(made.Name())

			return nil, fmt.Errorf("writing the key of the assistant's answers: %w", err)
		}

		err = place(made.Name(), path)

		os.Remove(made.Name())

		if err == nil {
			return key, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("writing the key of the assistant's answers: %w", err)
		}

		// Another start made one in this moment: read that one.
	}

	return nil, errors.New("the key of the assistant's answers could not be kept: something else keeps replacing it")
}

// link is os.Link; a variable so a test can be a file system without links.
var link = os.Link

/*
 * place puts the finished file at path only if nothing is there: by a
 * link, so that it appears whole. On a file system that has no links it
 * is written in place instead, which also fails when something is there
 * (and can be seen half written for a moment by another start, which then
 * takes it for "not a key" and makes its own: the later of the two wins
 * and the earlier is closed and started again, as the known limits say of
 * a copy that seals nothing).
 */
func place(made, path string) error {
	err := link(made, path)
	if err == nil || os.IsExist(err) {
		return err
	}

	raw, readErr := os.ReadFile(made)
	if readErr != nil {
		return readErr
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	_, err = f.Write(raw)
	if closed := f.Close(); err == nil {
		err = closed
	}

	if err != nil {
		os.Remove(path)
	}

	return err
}

/*
 * setAside takes away what is at path and is not a key, without taking a
 * key away by mistake: between looking and removing, another start may
 * have put a good key there. So the file is moved aside first and looked
 * at again where nobody else touches it; a good key is put back (unless a
 * newer one is there already), anything else is removed.
 */
func setAside(path string) error {
	aside, err := os.CreateTemp(filepath.Dir(path), FileName+".old-*")
	if err != nil {
		return err
	}

	aside.Close()

	if err := os.Rename(path, aside.Name()); err != nil {
		os.Remove(aside.Name())

		if os.IsNotExist(err) {
			return nil
		}

		// A folder with something in it, or anything else that will not
		// move: not ours to clear away.
		return err
	}

	if _, state := look(aside.Name()); state == whole {
		place(aside.Name(), path) // put back, unless a newer one is there
	}

	return os.Remove(aside.Name())
}

// Number is a new number for one request.
func Number() (string, error) {
	raw := make([]byte, numberBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

// numberLike is whether s has the shape Number gives.
func numberLike(s string) bool {
	if len(s) != 2*numberBytes {
		return false
	}

	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

/*
 * Seal is the seal on one answer: what was asked (number, method, path
 * with its query, body) and what was answered (status, body), under the key. Each part is
 * written with its length before it, so that no two different sets of
 * parts make the same bytes.
 */
func Seal(key []byte, number, method, path string, asked []byte, status int, answer []byte) string {
	mac := hmac.New(sha256.New, key)

	part := func(b []byte) {
		mac.Write([]byte(strconv.Itoa(len(b))))
		mac.Write([]byte{':'})
		mac.Write(b)
	}

	askedSum, answerSum := sha256.Sum256(asked), sha256.Sum256(answer)

	part([]byte("pn-scripts-assistant answer 1"))
	part([]byte(number))
	part([]byte(method))
	part([]byte(path))
	part(askedSum[:])
	part([]byte(strconv.Itoa(status)))
	part(answerSum[:])

	return hex.EncodeToString(mac.Sum(nil))
}

// Holds is whether seal is the seal of exactly this request and answer
// under key. With no key, no number or no seal it never is.
func Holds(key []byte, number, method, path string, asked []byte, status int, answer []byte, seal string) bool {
	if len(key) != keyBytes || !numberLike(number) || seal == "" {
		return false
	}

	want := Seal(key, number, method, path, asked, status, answer)

	return hmac.Equal([]byte(want), []byte(seal))
}

/*
 * Sealing puts the seal on the answer to a request that asked for one
 * (AskHeader), where key gives a key for that request. A request that did
 * not ask, or for which there is no key, passes through untouched, so the
 * page and everything else see no difference.
 *
 * The answer is held until it is whole: the seal covers all of it and goes
 * in a header, which leaves before the body does. That is why key should
 * give a key only for the few short answers that are asked for this way,
 * and never for one that streams (the server gives it for three routes);
 * and why an answer over MostAnswered is not sent at all.
 */
func Sealing(key func(*http.Request) []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number := r.Header.Get(AskHeader)
		if number == "" {
			next.ServeHTTP(w, r)

			return
		}

		k := key(r)
		if len(k) != keyBytes || !numberLike(number) {
			// Nothing to seal with, or not a number: answered as ever,
			// unsealed, and whoever asked for the seal believes none of it.
			next.ServeHTTP(w, r)

			return
		}

		var asked []byte

		if r.Body != nil {
			var err error

			asked, err = io.ReadAll(io.LimitReader(r.Body, MostAsked+1))

			switch {
			case err != nil:
				http.Error(w, "that request could not be read", http.StatusBadRequest)

				return
			case len(asked) > MostAsked:
				http.Error(w, "that request is too large to answer with a seal", http.StatusRequestEntityTooLarge)

				return
			}

			r.Body = io.NopCloser(bytes.NewReader(asked))
		}

		held := &holding{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(held, r)

		if held.over {
			http.Error(w, "that answer is too large to seal", http.StatusInternalServerError)

			return
		}

		w.Header().Set(SealHeader, Seal(k, number, r.Method, r.URL.RequestURI(), asked, held.status, held.body.Bytes()))
		w.Header().Del("Content-Length")
		w.WriteHeader(held.status)
		w.Write(held.body.Bytes())
	})
}

// holding keeps an answer until it is whole.
type holding struct {
	http.ResponseWriter

	status int
	wrote  bool
	over   bool
	body   bytes.Buffer
}

func (h *holding) WriteHeader(status int) {
	if !h.wrote {
		h.status, h.wrote = status, true
	}
}

func (h *holding) Write(b []byte) (int, error) {
	h.wrote = true

	if h.over || h.body.Len()+len(b) > MostAnswered {
		h.over = true
		h.body.Reset()

		return 0, errors.New("the answer is too large to seal")
	}

	return h.body.Write(b)
}

// Flush does nothing: the answer leaves when it is whole.
func (h *holding) Flush() {}

// Unwrap lets a handler move the connection's deadlines through this
// (http.ResponseController): a permission question waits minutes.
func (h *holding) Unwrap() http.ResponseWriter { return h.ResponseWriter }
