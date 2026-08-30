package mail

import (
	"strings"
	"testing"
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
