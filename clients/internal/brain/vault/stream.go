package vault

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

/*
 * Sealing a whole file as a stream, for the copy taken before a brain is
 * sealed in place: a database can be larger than is sensible to hold in
 * memory twice. Chunks of a megabyte, each sealed with its place and whether
 * it is the last bound into it, so a chunk moved, dropped or added is caught.
 */

var streamMagic = []byte("\x89PNSTREAM\x01")

const chunkSize = 1 << 20

// SealStream copies src to dst, sealed, and returns the plain SHA-256.
func SealStream(k *Key, dst io.Writer, src io.Reader) ([32]byte, error) {
	sum := sha256.New()

	if _, err := dst.Write(streamMagic); err != nil {
		return [32]byte{}, err
	}

	buf := make([]byte, chunkSize)
	next := make([]byte, chunkSize)

	n, err := io.ReadFull(src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return [32]byte{}, err
	}

	for index := uint64(0); ; index++ {
		m, nerr := io.ReadFull(src, next)
		if nerr != nil && nerr != io.ErrUnexpectedEOF && nerr != io.EOF {
			return [32]byte{}, nerr
		}

		last := m == 0

		sum.Write(buf[:n])

		head := make([]byte, 9)
		binary.BigEndian.PutUint64(head, index)

		if last {
			head[8] = 1
		}

		sealed := k.Seal(append(head, buf[:n]...))

		size := make([]byte, 4)
		binary.BigEndian.PutUint32(size, uint32(len(sealed)))

		if _, err := dst.Write(append(size, sealed...)); err != nil {
			return [32]byte{}, err
		}

		if last {
			break
		}

		buf, next, n = next, buf, m
	}

	var out [32]byte

	copy(out[:], sum.Sum(nil))

	return out, nil
}

// OpenStream reverses SealStream, refusing anything moved, cut or changed.
func OpenStream(k *Key, dst io.Writer, src io.Reader) error {
	r := bufio.NewReader(src)

	head := make([]byte, len(streamMagic))

	if _, err := io.ReadFull(r, head); err != nil || !bytes.Equal(head, streamMagic) {
		return errors.New("not a sealed copy")
	}

	for index := uint64(0); ; index++ {
		size := make([]byte, 4)

		if _, err := io.ReadFull(r, size); err != nil {
			return fmt.Errorf("the sealed copy ends early: %w", ErrTampered)
		}

		sealed := make([]byte, binary.BigEndian.Uint32(size))

		if _, err := io.ReadFull(r, sealed); err != nil {
			return ErrTampered
		}

		plain, err := k.Open(sealed)
		if err != nil {
			return err
		}

		if len(plain) < 9 || binary.BigEndian.Uint64(plain) != index {
			return ErrTampered
		}

		if _, err := dst.Write(plain[9:]); err != nil {
			return err
		}

		if plain[8] == 1 {
			if _, err := r.ReadByte(); err != io.EOF {
				return ErrTampered
			}

			return nil
		}
	}
}

/*
 * SealedCopy writes a sealed copy of a file and proves it: the copy is opened
 * again and must come back to exactly the same bytes, and only then is it
 * synced and moved into place.
 */
func SealedCopy(k *Key, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	temp := dst + ".new"

	out, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	want, err := SealStream(k, out, in)
	if err == nil {
		err = out.Sync()
	}

	if cerr := out.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		os.Remove(temp)

		return err
	}

	check, err := os.Open(temp)
	if err != nil {
		return err
	}

	got := sha256.New()
	err = OpenStream(k, got, check)
	check.Close()

	if err != nil || !bytes.Equal(got.Sum(nil), want[:]) {
		os.Remove(temp)

		return fmt.Errorf("the sealed copy of %s did not read back the same; nothing was changed", src)
	}

	return os.Rename(temp, dst)
}

/*
 * LineSealer seals each line written through it with the key in force, so a
 * log on disk says nothing to anybody without the key. Before a brain is
 * unlocked there is no key and a line goes as it is — those are the lines
 * about starting, which hold nothing of anybody's.
 */
type LineSealer struct {
	W io.Writer

	mu      sync.Mutex
	pending []byte
}

func (l *LineSealer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.pending = append(l.pending, p...)

	for {
		i := bytes.IndexByte(l.pending, '\n')
		if i < 0 {
			break
		}

		line := string(l.pending[:i])
		l.pending = l.pending[i+1:]

		if k := Active(); k != nil {
			line = k.SealString(line)
		}

		if _, err := io.WriteString(l.W, line+"\n"); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

// OpenLines is a log as it was written, each sealed line opened; lines that
// cannot be opened are said to be so rather than dropped.
func OpenLines(k *Key, text string) string {
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if !IsSealedString(line) {
			continue
		}

		if plain, err := k.OpenString(line); err == nil {
			lines[i] = plain
		} else {
			lines[i] = "[a line that could not be opened: " + err.Error() + "]"
		}
	}

	return strings.Join(lines, "\n")
}
