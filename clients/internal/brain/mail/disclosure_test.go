package mail

import (
	"net/mail"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/aidisclosure"
)

/*
 * Every message this program sends was written with its model and approved
 * by its owner, so every one carries the machine-readable AI mark (EU AI Act
 * article 50(2)) in its headers, and the body, the owner's approved words,
 * is left exactly as it was.
 */
func TestASentMessageIsMarkedAsWrittenWithAnAI(t *testing.T) {
	body := "Здравейте,\nдоговорът е готов.\n"
	message := compose("me@example.com", []string{"you@example.com"}, "Договор", body)

	parsed, err := mail.ReadMessage(strings.NewReader(message))
	if err != nil {
		t.Fatalf("the message does not parse: %v", err)
	}

	if got := parsed.Header.Get(aidisclosure.MailHeader); got != aidisclosure.MailValue {
		t.Errorf("%s = %q, want %q", aidisclosure.MailHeader, got, aidisclosure.MailValue)
	}

	if strings.Count(message, aidisclosure.MailHeader+":") != 1 {
		t.Errorf("the mark is not there exactly once:\n%s", message)
	}

	head, rest, _ := strings.Cut(message, "\r\n\r\n")
	if !strings.Contains(head, aidisclosure.MailHeader) {
		t.Error("the mark is not in the headers")
	}

	if rest != strings.ReplaceAll(body, "\n", "\r\n") {
		t.Errorf("the body changed: %q", rest)
	}
}
