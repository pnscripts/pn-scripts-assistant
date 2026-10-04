package mail

import (
	"errors"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"
)

/*
 * A subject that is not ASCII has to be encoded.
 *
 * A raw non-ASCII header is not legal and every server mangles it differently.
 * This is the ordinary case here rather than the exception: the owner of this
 * brain writes in Bulgarian.
 */
func TestASubjectInAnotherAlphabet(t *testing.T) {
	message := compose("me@example.com", []string{"you@example.com"},
		"Добър ден", "Здравей,\nкак си?\n")

	subject := headerOf(t, message, "Subject")

	if strings.Contains(subject, "Добър") {
		t.Errorf("the subject went out raw: %q", subject)
	}

	if !strings.HasPrefix(subject, "=?utf-8?") {
		t.Errorf("the subject was not encoded: %q", subject)
	}

	// And the body says it is UTF-8, so the words survive.
	if !strings.Contains(message, "charset=utf-8") {
		t.Error("the message does not declare its encoding")
	}

	if !strings.Contains(message, "Здравей") {
		t.Error("the body lost its text")
	}
}

// Every line ends the way a message's lines have to end.
func TestLineEndings(t *testing.T) {
	message := compose("me@example.com", []string{"you@example.com"},
		"Hello", "one\ntwo\r\nthree")

	if strings.Contains(strings.ReplaceAll(message, "\r\n", ""), "\n") {
		t.Error("a bare newline survived into the message")
	}

	// The blank line between headers and body has to be there, or the body
	// becomes more headers.
	if !strings.Contains(message, "\r\n\r\none") {
		t.Errorf("no blank line before the body: %q", message)
	}
}

// Addresses are checked before connecting, so the failure names the address
// rather than arriving as a numeric code from a server.
func TestABadAddressIsCaughtHere(t *testing.T) {
	a := Account{Host: "mail.example.com", User: "me@example.com", Password: "x"}

	if _, err := Send(a, []string{"not an address"}, "hi", "there"); err == nil {
		t.Fatal("a message was sent to something that is not an address")
	} else if !strings.Contains(err.Error(), "not an email address") {
		t.Errorf("unhelpful error: %v", err)
	}
}

// With nothing set up, the error says what to do rather than failing to connect
// to an empty host name.
func TestNoMailboxSaysWhatToDo(t *testing.T) {
	if _, err := Send(Account{}, []string{"you@example.com"}, "hi", "x"); err == nil {
		t.Fatal("sending worked with no account")
	} else if !strings.Contains(err.Error(), "Settings") {
		t.Errorf("the error does not say where to set it up: %v", err)
	}

	if _, err := Recent(Account{}, "INBOX", 5); err == nil {
		t.Fatal("reading worked with no account")
	}
}

/*
 * A literal is not a line.
 *
 * A server announces {1234} and then sends exactly that many bytes, newlines
 * and all. A reader that goes line by line without honouring it mistakes the
 * middle of somebody's email for a protocol response — and then acts on it.
 */
func TestHeadersAreReadOutOfAFetchReply(t *testing.T) {
	lines := []string{
		"* 12 FETCH (BODY[HEADER.FIELDS (FROM SUBJECT DATE)]",
		"From: Anna <anna@example.com>\nSubject: =?utf-8?B?0JTQvtCx0YrRgA==?=\nDate: Mon, 1 Sep 2026 10:00:00 +0300\n",
		"* 13 FETCH (BODY[HEADER.FIELDS (FROM SUBJECT DATE)]",
		"From: Bob <bob@example.com>\nSubject: Lunch\n",
	}

	got := parseHeaders(lines)

	if len(got) != 2 {
		t.Fatalf("read %d messages out of two: %+v", len(got), got)
	}

	if got[0].Number != 12 || !strings.Contains(got[0].From, "anna@example.com") {
		t.Errorf("first message wrong: %+v", got[0])
	}

	// The encoded subject has to come back as words, or it is read aloud as
	// question marks and equals signs.
	if got[0].Subject != "Добър" {
		t.Errorf("the subject was not decoded: %q", got[0].Subject)
	}

	if got[1].Subject != "Lunch" {
		t.Errorf("second message wrong: %+v", got[1])
	}
}

// headerOf pulls one header out of a composed message.
func headerOf(t *testing.T, message, name string) string {
	t.Helper()

	for _, line := range strings.Split(message, "\r\n") {
		if strings.HasPrefix(line, name+": ") {
			return strings.TrimPrefix(line, name+": ")
		}
	}

	t.Fatalf("no %s header in %q", name, message)

	return ""
}

/*
 * Where mail goes out when nobody said.
 *
 * Gmail reads on imap.gmail.com and sends on smtp.gmail.com; sending to the
 * reading server on 587 fails. Anything not named imap.<domain> is not guessed
 * at, and the error says which field to fill in.
 */
func TestTheSendingServerIsWorkedOut(t *testing.T) {
	cases := []struct {
		host, smtp, want string
	}{
		{"imap.gmail.com", "", "smtp.gmail.com"},
		{"IMAP.Example.org", "", "smtp.Example.org"},
		{"imap.gmail.com", "smtp.relay.example.com", "smtp.relay.example.com"},
		{"mail.example.com", " smtp.example.com ", "smtp.example.com"},
	}

	for _, c := range cases {
		got, err := sendingHost(Account{Host: c.host, SMTPHost: c.smtp})
		if err != nil {
			t.Errorf("%q/%q: %v", c.host, c.smtp, err)
		} else if got != c.want {
			t.Errorf("%q/%q: sent through %q, want %q", c.host, c.smtp, got, c.want)
		}
	}

	for _, host := range []string{"mail.example.com", "imap.", "outlook.office365.com"} {
		if got, err := sendingHost(Account{Host: host}); err == nil {
			t.Errorf("%q was guessed as %q rather than asked for", host, got)
		} else if !strings.Contains(err.Error(), "Outgoing server (SMTP)") {
			t.Errorf("the error does not say what to fill in: %v", err)
		}
	}
}

// Every message carries a Date and a Message-ID in the sender's domain, and no
// two messages share a Message-ID.
func TestAMessageIsDatedAndNamed(t *testing.T) {
	first := compose("Petar <me@example.com>", []string{"you@example.org"}, "Hello", "hi")
	second := compose("me@example.com", []string{"you@example.org"}, "Hello", "hi")

	if _, err := mail.ParseDate(headerOf(t, first, "Date")); err != nil {
		t.Errorf("the Date header does not parse: %v", err)
	}

	id := headerOf(t, first, "Message-ID")
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@example.com>") ||
		strings.Count(id, "@") != 1 || strings.ContainsAny(id, " \t") {
		t.Errorf("not a Message-ID in the sender's domain: %q", id)
	}

	if other := headerOf(t, second, "Message-ID"); other == id {
		t.Errorf("two messages share the Message-ID %q", id)
	}
}

/*
 * A server that answers the connection and then says nothing does not hold
 * sending forever.
 */
func TestASilentServerTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Accept and hold every connection without ever sending a greeting.
	held := make(chan net.Conn, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			held <- conn
		}
	}()
	defer func() {
		close(held)
		for conn := range held {
			conn.Close()
		}
	}()

	defer func(d, s time.Duration) { dialTimeout, sessionTimeout = d, s }(dialTimeout, sessionTimeout)
	dialTimeout, sessionTimeout = time.Second, 200*time.Millisecond

	_, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)

	done := make(chan error, 1)
	go func() {
		done <- deliver("127.0.0.1", portNumber, "me@example.com", "x",
			"me@example.com", []string{"you@example.com"}, []byte("hi"))
	}()

	select {
	case err := <-done:
		var timeout net.Error
		if err == nil {
			t.Fatal("a server that never answered took the message")
		} else if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Errorf("failed, but not by timing out: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sending was still waiting on a silent server after five seconds")
	}
}
