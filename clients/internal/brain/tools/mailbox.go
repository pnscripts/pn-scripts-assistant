package tools

import (
	"pn-scripts-assistant/internal/brain/scam"

	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

/*
 * Reading Thunderbird's own copy of its owner's mail.
 *
 * read_email speaks IMAP to one account configured in settings. Thunderbird
 * already holds every account its owner reads, in plain mbox files on this
 * disk, and reading them needs no password and nothing leaving the machine —
 * which is how the mail behind a complaint or a reviewer's thread was found
 * by hand (ADR 0007).
 *
 * Read only. Thunderbird owns these files; a message is "deleted" in one
 * until Thunderbird compacts it, so those are skipped by their status flag.
 */

// ReadMailbox is the tool.
type ReadMailbox struct {
	// Home is where to look for Thunderbird; empty means the user's home.
	Home string

	// Lookup asks whether a domain exists, for the scam check; nil when
	// nothing may be asked of the network, and then only the words and the
	// addresses are checked.
	Lookup func() scam.Lookup
}

func (ReadMailbox) Name() string { return "read_mailbox" }

func (ReadMailbox) Description() string {
	return "Read the owner's mail from Thunderbird's local folders, with no password and nothing sent " +
		"anywhere. Actions: \"folders\" lists every folder of every account; \"list\" {folder, limit, " +
		"search} lists messages newest first with their numbers; \"read\" {folder, number} reads one."
}

func (ReadMailbox) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["folders", "list", "read"]},
			"folder": {"type": "string", "description": "A folder as folders lists it"},
			"limit": {"type": "integer", "description": "How many to list, default 20"},
			"search": {"type": "string", "description": "Only messages whose sender or subject contain this"},
			"number": {"type": "integer", "description": "A message number from list"}
		},
		"required": ["action"]
	}`)
}

func (ReadMailbox) Risk() Risk { return Safe }

type mailboxArgs struct {
	Action string `json:"action"`
	Folder string `json:"folder"`
	Limit  int    `json:"limit"`
	Search string `json:"search"`
	Number int    `json:"number"`
}

func (ReadMailbox) Summarize(raw json.RawMessage) string {
	var a mailboxArgs
	argsOf(raw, &a)

	switch a.Action {
	case "list":
		return "List mail in Thunderbird's " + a.Folder
	case "read":
		return fmt.Sprintf("Read message %d in Thunderbird's %s", a.Number, a.Folder)
	}

	return "List Thunderbird's mail folders"
}

func (ReadMailbox) Cues() []string {
	return []string{"thunderbird", "mailbox", "mail", "email", "inbox", "message", "messages", "letter",
		"complaint", "reviewer", "thread"}
}

// MostMessageText bounds one message handed back.
const MostMessageText = 15000

func (m ReadMailbox) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a mailboxArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	profiles := m.profiles()
	if len(profiles) == 0 {
		return "", errors.New("there is no Thunderbird profile on this machine")
	}

	if a.Action == "" || a.Action == "folders" {
		var all []string

		for _, p := range profiles {
			all = append(all, mailFolders(p)...)
		}

		if len(all) == 0 {
			return "Thunderbird has no local mail folders yet.", nil
		}

		return strings.Join(all, "\n"), nil
	}

	file, err := m.folderFile(profiles, a.Folder)
	if err != nil {
		return "", err
	}

	starts, err := messageStarts(file)
	if err != nil {
		return "", err
	}

	switch a.Action {
	case "list":
		limit := a.Limit
		if limit <= 0 {
			limit = 20
		}

		needle := strings.ToLower(strings.TrimSpace(a.Search))

		var lines []string

		for i := len(starts) - 1; i >= 0 && len(lines) < limit; i-- {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}

			h, deleted, err := headersAt(file, starts[i])
			if err != nil || deleted {
				continue
			}

			from, subject := decodeHeader(h.Get("From")), decodeHeader(h.Get("Subject"))

			if needle != "" && !strings.Contains(strings.ToLower(from+" "+subject), needle) {
				continue
			}

			line := fmt.Sprintf("%d. %s — %s — %s", i+1, h.Get("Date"), from, subject)

			// A first look from the headers alone; reading the message
			// checks it properly.
			if v := scam.Check(scam.Message{From: from, ReplyTo: decodeHeader(h.Get("Reply-To")), Subject: subject}, nil); v.Warns() {
				line += " [scam check: " + v.Level + "]"
			}

			lines = append(lines, line)
		}

		if len(lines) == 0 {
			return "Nothing matched.", nil
		}

		return fmt.Sprintf("%s: %d messages, newest first\n%s", a.Folder, len(starts),
			strings.Join(lines, "\n")), nil

	case "read":
		if a.Number < 1 || a.Number > len(starts) {
			return "", fmt.Errorf("there is no message %d; the folder holds %d", a.Number, len(starts))
		}

		var look scam.Lookup
		if m.Lookup != nil {
			look = m.Lookup()
		}

		return readMessageAt(file, starts, a.Number-1, look)
	}

	return "", fmt.Errorf("there is no action %q", a.Action)
}

// profiles are Thunderbird's profile folders, wherever it was installed from.
func (m ReadMailbox) profiles() []string {
	home := m.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	var out []string

	for _, base := range []string{
		filepath.Join(home, ".thunderbird"),
		filepath.Join(home, "snap", "thunderbird", "common", ".thunderbird"),
		filepath.Join(home, ".var", "app", "org.mozilla.Thunderbird", ".thunderbird"),
	} {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}

		for _, e := range entries {
			dir := filepath.Join(base, e.Name())

			if !e.IsDir() {
				continue
			}

			if _, err := os.Stat(filepath.Join(dir, "prefs.js")); err == nil {
				out = append(out, dir)
			}
		}
	}

	sort.Strings(out)

	return out
}

/*
 * mailFolders are the mbox files in a profile, named by where they sit:
 * "ImapMail/imap.example.com/INBOX". A folder is a file with a .msf index
 * beside it; subfolders live in a ".sbd" directory of the same name.
 */
func mailFolders(profile string) []string {
	var out []string

	for _, top := range []string{"Mail", "ImapMail"} {
		filepath.WalkDir(filepath.Join(profile, top), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasSuffix(path, ".msf") {
				return nil
			}

			if _, err := os.Stat(path + ".msf"); err != nil {
				return nil
			}

			rel, _ := filepath.Rel(profile, path)
			out = append(out, strings.ReplaceAll(filepath.ToSlash(rel), ".sbd/", "/"))

			return nil
		})
	}

	return out
}

func (m ReadMailbox) folderFile(profiles []string, folder string) (string, error) {
	folder = strings.Trim(filepath.ToSlash(strings.TrimSpace(folder)), "/")
	if folder == "" {
		return "", errors.New("which folder? use folders to list them")
	}

	for _, p := range profiles {
		for _, f := range mailFolders(p) {
			if f == folder {
				parts := strings.Split(f, "/")

				// Back from the shown name to the file: every folder but the
				// first two levels sits in a .sbd directory.
				for i := 2; i < len(parts)-1; i++ {
					parts[i] += ".sbd"
				}

				return filepath.Join(append([]string{p}, parts...)...), nil
			}
		}
	}

	return "", fmt.Errorf("there is no folder %q; use folders to list them", folder)
}

// messageStarts is the offset of every "From " line that begins a message.
func messageStarts(file string) ([]int64, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<16)

	var (
		out        []int64
		offset     int64
		afterBlank = true
	)

	for {
		line, err := r.ReadSlice('\n')

		if err == bufio.ErrBufferFull {
			// A very long line: read the rest of it and carry on.
			n := int64(len(line))

			for err == bufio.ErrBufferFull {
				line, err = r.ReadSlice('\n')
				n += int64(len(line))
			}

			offset += n
			afterBlank = false

			if err != nil {
				break
			}

			continue
		}

		if afterBlank && bytes.HasPrefix(line, []byte("From ")) {
			out = append(out, offset)
		}

		afterBlank = len(bytes.TrimRight(line, "\r\n")) == 0
		offset += int64(len(line))

		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, err
		}
	}

	return out, nil
}

// headersAt reads the headers of the message starting at offset, and whether
// Thunderbird has marked it deleted.
func headersAt(file string, offset int64) (mail.Header, bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, false, err
	}

	r := bufio.NewReader(io.LimitReader(f, 256<<10))

	r.ReadString('\n') // the "From " separator

	msg, err := mail.ReadMessage(r)
	if err != nil {
		return nil, false, err
	}

	return msg.Header, deletedFlag(msg.Header), nil
}

func deletedFlag(h mail.Header) bool {
	status, err := strconv.ParseUint(strings.TrimSpace(h.Get("X-Mozilla-Status")), 16, 32)

	return err == nil && status&0x0008 != 0
}

func readMessageAt(file string, starts []int64, i int, look scam.Lookup) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	end := int64(-1)
	if i+1 < len(starts) {
		end = starts[i+1]
	}

	if _, err := f.Seek(starts[i], io.SeekStart); err != nil {
		return "", err
	}

	var src io.Reader = f
	if end > 0 {
		src = io.LimitReader(f, end-starts[i])
	}

	r := bufio.NewReader(io.LimitReader(src, 32<<20))
	r.ReadString('\n')

	msg, err := mail.ReadMessage(r)
	if err != nil {
		return "", err
	}

	body := bodyText(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"), msg.Body)

	var b strings.Builder

	for _, k := range []string{"Date", "From", "Reply-To", "To", "Cc", "Subject", "Message-ID"} {
		if v := msg.Header.Get(k); v != "" {
			b.WriteString(k + ": " + decodeHeader(v) + "\n")
		}
	}

	b.WriteString("\n" + truncateRunes(strings.TrimSpace(body), MostMessageText))

	// Every message read is checked for the usual tricks, and the result is
	// written where the model and its owner both see it.
	verdict := scam.Check(scam.Message{
		From:    decodeHeader(msg.Header.Get("From")),
		ReplyTo: decodeHeader(msg.Header.Get("Reply-To")),
		Subject: decodeHeader(msg.Header.Get("Subject")),
		Body:    body,
	}, look)

	b.WriteString("\n\n" + verdict.String())

	return b.String(), nil
}

// bodyText is the readable text of a body: the plain part when there is one,
// the HTML part made readable when not.
func bodyText(contentType, encoding string, body io.Reader) string {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		media = "text/plain"
	}

	if strings.HasPrefix(media, "multipart/") && params["boundary"] != "" {
		parts := multipart.NewReader(body, params["boundary"])

		var plain, html string

		for {
			p, err := parts.NextRawPart()
			if err != nil {
				break
			}

			text := bodyText(p.Header.Get("Content-Type"), p.Header.Get("Content-Transfer-Encoding"), p)

			ct := strings.ToLower(p.Header.Get("Content-Type"))

			switch {
			case strings.HasPrefix(ct, "multipart/") && plain == "":
				plain = text
			case strings.HasPrefix(ct, "text/plain") || ct == "":
				if plain == "" {
					plain = text
				}
			case strings.HasPrefix(ct, "text/html"):
				if html == "" {
					html = text
				}
			}
		}

		if strings.TrimSpace(plain) != "" {
			return plain
		}

		return html
	}

	var r io.Reader = body

	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		r = quotedprintable.NewReader(body)
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, &newlineless{body})
	}

	raw, _ := io.ReadAll(io.LimitReader(r, 8<<20))

	if strings.HasPrefix(media, "text/html") {
		return ReadableText(string(raw))
	}

	if strings.HasPrefix(media, "text/") {
		return string(raw)
	}

	return ""
}

// newlineless drops line breaks, which base64 bodies are full of.
type newlineless struct{ r io.Reader }

func (n *newlineless) Read(p []byte) (int, error) {
	k, err := n.r.Read(p)

	out := p[:0]

	for _, c := range p[:k] {
		if c != '\r' && c != '\n' {
			out = append(out, c)
		}
	}

	return len(out), err
}

var headerWords = &mime.WordDecoder{
	// Charsets Go does not know are handed back as they are rather than
	// failing the whole header.
	CharsetReader: func(_ string, input io.Reader) (io.Reader, error) { return input, nil },
}

func decodeHeader(v string) string {
	if out, err := headerWords.DecodeHeader(v); err == nil {
		return out
	}

	return v
}
