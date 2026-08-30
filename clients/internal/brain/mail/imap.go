// Package mail reads and sends the owner's email.
//
// Written against crypto/tls and bufio rather than an IMAP library, because
// this module has one dependency — SQLite — and adding a second to read a
// mailbox is a poor trade. IMAP is an old, chatty, line-based protocol, and the
// part of it needed to list recent messages and read one is small.
//
// Credentials are never held here. They are read from the settings file, which
// its owner fills in themselves, and this package is handed them for the length
// of one connection.
package mail

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"strconv"
	"strings"
	"time"
)

// Account is what is needed to reach a mailbox.
type Account struct {
	Host     string
	Port     int
	User     string
	Password string

	// From is the address to send as, when it differs from the user name.
	From string

	// SMTPHost and SMTPPort are for sending. Empty means the same host on 587.
	SMTPHost string
	SMTPPort int
}

// Configured reports whether there is enough to try.
func (a Account) Configured() bool {
	return a.Host != "" && a.User != "" && a.Password != ""
}

// Message is one email, as much of it as was asked for.
type Message struct {
	Number  int
	From    string
	Subject string
	Date    string
	Body    string
}

// client is one IMAP connection.
type client struct {
	conn net.Conn
	r    *bufio.Reader
	tag  int
}

// dial opens a TLS connection and reads the greeting.
//
// TLS from the first byte, on 993. The alternative — connecting in the clear
// and upgrading with STARTTLS — is one misconfigured server away from sending
// somebody's password across the network in plain text, so it is not offered.
func dial(a Account, timeout time.Duration) (*client, error) {
	port := a.Port
	if port == 0 {
		port = 993
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: timeout},
		"tcp", net.JoinHostPort(a.Host, strconv.Itoa(port)),
		&tls.Config{ServerName: a.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", a.Host, err)
	}

	_ = conn.SetDeadline(time.Now().Add(timeout))

	c := &client{conn: conn, r: bufio.NewReaderSize(conn, 64<<10)}

	// The greeting, which must be an untagged OK.
	line, err := c.r.ReadString('\n')
	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("no greeting from %s: %w", a.Host, err)
	}

	if !strings.HasPrefix(line, "* OK") {
		conn.Close()

		return nil, fmt.Errorf("%s did not greet as an IMAP server: %s",
			a.Host, strings.TrimSpace(line))
	}

	return c, nil
}

func (c *client) close() {
	if c.conn != nil {
		_, _ = c.send("LOGOUT")
		c.conn.Close()
	}
}

/*
 * send writes one command and collects everything up to its tagged reply.
 *
 * IMAP replies are lines, except when they are not: a server may announce a
 * literal as {1234} at the end of a line and then send exactly that many bytes,
 * newlines and all. Reading line by line without honouring that is how a parser
 * mistakes the middle of somebody's email for a protocol response.
 */
func (c *client) send(format string, args ...any) ([]string, error) {
	c.tag++

	tag := fmt.Sprintf("a%03d", c.tag)
	command := fmt.Sprintf(format, args...)

	if _, err := fmt.Fprintf(c.conn, "%s %s\r\n", tag, command); err != nil {
		return nil, err
	}

	var lines []string

	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return lines, err
		}

		line = strings.TrimRight(line, "\r\n")

		// A literal: read exactly that many bytes and keep them as one line.
		if open := strings.LastIndex(line, "{"); open >= 0 && strings.HasSuffix(line, "}") {
			if n, err := strconv.Atoi(line[open+1 : len(line)-1]); err == nil && n >= 0 {
				body := make([]byte, n)

				if _, err := io.ReadFull(c.r, body); err != nil {
					return lines, err
				}

				lines = append(lines, line[:open], string(body))

				continue
			}
		}

		if strings.HasPrefix(line, tag+" ") {
			answer := strings.TrimPrefix(line, tag+" ")

			if !strings.HasPrefix(answer, "OK") {
				return lines, fmt.Errorf("%s", strings.TrimSpace(answer))
			}

			return lines, nil
		}

		lines = append(lines, line)
	}
}

// login authenticates.
func (c *client) login(user, password string) error {
	// Quoted, because passwords contain spaces and quotes and IMAP is
	// whitespace-separated.
	_, err := c.send(`LOGIN %s %s`, quote(user), quote(password))
	if err != nil {
		return fmt.Errorf("signing in as %s: %w", user, err)
	}

	return nil
}

// selectBox opens a mailbox and reports how many messages are in it.
func (c *client) selectBox(name string) (int, error) {
	lines, err := c.send("SELECT %s", quote(name))
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", name, err)
	}

	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) >= 3 &&
			fields[0] == "*" && strings.EqualFold(fields[2], "EXISTS") {
			n, _ := strconv.Atoi(fields[1])

			return n, nil
		}
	}

	return 0, nil
}

// Recent lists the newest messages in a mailbox, newest first.
func Recent(a Account, box string, count int) ([]Message, error) {
	if !a.Configured() {
		return nil, errNotSetUp
	}

	if box == "" {
		box = "INBOX"
	}

	c, err := dial(a, 30*time.Second)
	if err != nil {
		return nil, err
	}

	defer c.close()

	if err := c.login(a.User, a.Password); err != nil {
		return nil, err
	}

	total, err := c.selectBox(box)
	if err != nil {
		return nil, err
	}

	if total == 0 {
		return nil, nil
	}

	from := total - count + 1
	if from < 1 {
		from = 1
	}

	// PEEK, so that looking at the list does not mark anybody's mail as read.
	lines, err := c.send(
		"FETCH %d:%d (BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE)])", from, total)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", box, err)
	}

	out := parseHeaders(lines)

	// Newest first, which is the order anybody means by "my last few emails".
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return out, nil
}

// Read returns one message, with its text.
func Read(a Account, box string, number int) (Message, error) {
	if !a.Configured() {
		return Message{}, errNotSetUp
	}

	if box == "" {
		box = "INBOX"
	}

	c, err := dial(a, 60*time.Second)
	if err != nil {
		return Message{}, err
	}

	defer c.close()

	if err := c.login(a.User, a.Password); err != nil {
		return Message{}, err
	}

	if _, err := c.selectBox(box); err != nil {
		return Message{}, err
	}

	lines, err := c.send("FETCH %d (BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE)] BODY.PEEK[TEXT])",
		number)
	if err != nil {
		return Message{}, fmt.Errorf("reading message %d: %w", number, err)
	}

	found := parseHeaders(lines)

	msg := Message{Number: number}

	if len(found) > 0 {
		msg = found[0]
		msg.Number = number
	}

	// The body is the longest literal that is not the header block.
	for _, line := range lines {
		if len(line) > len(msg.Body) && strings.Count(line, "\n") > 0 &&
			!strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "from:") {
			msg.Body = line
		}
	}

	msg.Body = strings.TrimSpace(msg.Body)

	return msg, nil
}

// parseHeaders reads the header blocks out of a FETCH reply.
func parseHeaders(lines []string) []Message {
	var (
		out     []Message
		current Message
		open    bool
	)

	for _, line := range lines {
		// "* 12 FETCH (BODY[HEADER.FIELDS ..." starts a message.
		if fields := strings.Fields(line); len(fields) >= 3 &&
			fields[0] == "*" && strings.EqualFold(fields[2], "FETCH") {
			if open {
				out = append(out, current)
			}

			n, _ := strconv.Atoi(fields[1])
			current = Message{Number: n}
			open = true

			continue
		}

		if !open || !strings.Contains(line, ":") {
			continue
		}

		for _, header := range strings.Split(line, "\n") {
			name, value, found := strings.Cut(header, ":")
			if !found {
				continue
			}

			value = decodeWord(strings.TrimSpace(value))

			switch strings.ToLower(strings.TrimSpace(name)) {
			case "from":
				current.From = value
			case "subject":
				current.Subject = value
			case "date":
				current.Date = value
			}
		}
	}

	if open {
		out = append(out, current)
	}

	return out
}

/*
 * decodeWord turns an encoded header into readable text.
 *
 * Subjects that are not plain ASCII arrive as =?UTF-8?B?...?= and reading them
 * out loud as that is worse than useless — which matters here, because the
 * owner of this brain writes in Bulgarian and every one of their subjects
 * arrives encoded.
 */
func decodeWord(text string) string {
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(text)
	if err != nil {
		return text
	}

	return decoded
}

// quote makes a string safe to send as an IMAP argument.
func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)

	return `"` + r.Replace(s) + `"`
}

var errNotSetUp = fmt.Errorf(
	"no mailbox is set up: put the address, server and an app password into " +
		"Settings first")
