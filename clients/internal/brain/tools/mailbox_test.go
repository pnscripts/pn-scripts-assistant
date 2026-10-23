package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/scam"
)

/*
 * A Thunderbird profile laid out the way Thunderbird lays it out — a folder
 * file with its .msf index, a subfolder in a .sbd directory — holding a plain
 * message, one Thunderbird has marked deleted, and a multipart one with an
 * encoded subject and a quoted-printable body.
 */
func fakeThunderbird(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	profile := filepath.Join(home, ".thunderbird", "abcd.default-release")
	server := filepath.Join(profile, "ImapMail", "imap.example.com")

	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(os.MkdirAll(filepath.Join(server, "INBOX.sbd"), 0o700))
	must(os.WriteFile(filepath.Join(profile, "prefs.js"), []byte("// prefs\n"), 0o600))

	inbox := "From - Mon Oct 06 10:00:00 2026\n" +
		"X-Mozilla-Status: 0001\n" +
		"From: Grid Operator <notices@grid.example>\n" +
		"Subject: Planned interruption\n" +
		"Date: Mon, 6 Oct 2026 10:00:00 +0300\n" +
		"\n" +
		"Power will be off from 09:00 to 13:00.\n" +
		"\n" +
		"From - Tue Oct 07 10:00:00 2026\n" +
		"X-Mozilla-Status: 0009\n" +
		"From: Spam <spam@example.com>\n" +
		"Subject: deleted one\n" +
		"\n" +
		"gone\n" +
		"\n" +
		"From - Wed Oct 08 10:00:00 2026\n" +
		"X-Mozilla-Status: 0001\n" +
		"From: Plugin Review <reviews@example.org>\n" +
		"Subject: =?UTF-8?B?0J/RgNC+0LLQtdGA0LrQsA==?= of your plugin\n" +
		"Date: Wed, 8 Oct 2026 10:00:00 +0300\n" +
		"MIME-Version: 1.0\n" +
		"Content-Type: multipart/alternative; boundary=\"XX\"\n" +
		"\n" +
		"--XX\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"Content-Transfer-Encoding: quoted-printable\n" +
		"\n" +
		"Please change the plugin name =E2=80=94 it is too generic.\n" +
		"--XX\n" +
		"Content-Type: text/html\n" +
		"\n" +
		"<p>html version</p>\n" +
		"--XX--\n"

	must(os.WriteFile(filepath.Join(server, "INBOX"), []byte(inbox), 0o600))
	must(os.WriteFile(filepath.Join(server, "INBOX.msf"), nil, 0o600))
	must(os.WriteFile(filepath.Join(server, "INBOX.sbd", "Work"), []byte("From - x\nSubject: w\n\nbody\n"), 0o600))
	must(os.WriteFile(filepath.Join(server, "INBOX.sbd", "Work.msf"), nil, 0o600))
	must(os.WriteFile(filepath.Join(server, "msgFilterRules.dat"), []byte("x"), 0o600))

	return home
}

func mailbox(t *testing.T, m ReadMailbox, args string) string {
	t.Helper()

	out, err := m.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: %v", args, err)
	}

	return out
}

func TestThunderbirdsMailIsRead(t *testing.T) {
	m := ReadMailbox{Home: fakeThunderbird(t)}

	folders := mailbox(t, m, `{"action":"folders"}`)
	if !strings.Contains(folders, "ImapMail/imap.example.com/INBOX\n") ||
		!strings.Contains(folders, "ImapMail/imap.example.com/INBOX/Work") || strings.Contains(folders, "msgFilter") {
		t.Fatalf("folders:\n%s", folders)
	}

	list := mailbox(t, m, `{"action":"list","folder":"ImapMail/imap.example.com/INBOX"}`)

	if strings.Contains(list, "deleted one") {
		t.Errorf("a message Thunderbird deleted was listed:\n%s", list)
	}

	review, outage := strings.Index(list, "Проверка of your plugin"), strings.Index(list, "Planned interruption")
	if review < 0 || outage < 0 || review > outage {
		t.Errorf("not newest first, or the subject was not decoded:\n%s", list)
	}

	if !strings.Contains(list, "3. ") || !strings.Contains(list, "1. ") {
		t.Errorf("the numbers are not the messages' places:\n%s", list)
	}

	searched := mailbox(t, m, `{"action":"list","folder":"ImapMail/imap.example.com/INBOX","search":"grid"}`)
	if !strings.Contains(searched, "Planned interruption") || strings.Contains(searched, "plugin") {
		t.Errorf("search:\n%s", searched)
	}

	read := mailbox(t, m, `{"action":"read","folder":"ImapMail/imap.example.com/INBOX","number":3}`)
	if !strings.Contains(read, "plugin name — it is too generic") || strings.Contains(read, "html version") {
		t.Errorf("the plain part was not read and decoded:\n%s", read)
	}

	sub := mailbox(t, m, `{"action":"read","folder":"ImapMail/imap.example.com/INBOX/Work","number":1}`)
	if !strings.Contains(sub, "body") {
		t.Errorf("a subfolder was not read:\n%s", sub)
	}

	if _, err := m.Execute(context.Background(), json.RawMessage(`{"action":"read","folder":"ImapMail/imap.example.com/INBOX","number":9}`)); err == nil {
		t.Error("a message that does not exist was read")
	}
}

func TestNoThunderbirdIsSaid(t *testing.T) {
	_, err := ReadMailbox{Home: t.TempDir()}.Execute(context.Background(), json.RawMessage(`{"action":"folders"}`))
	if err == nil || !strings.Contains(err.Error(), "no Thunderbird") {
		t.Errorf("got %v", err)
	}
}

// fakeLookup answers from a list, so the test never asks the network.
type fakeLookup map[string]bool

func (f fakeLookup) Resolves(d string) (bool, bool) { v, ok := f[d]; return v, ok }

func TestEveryMessageReadIsCheckedForScams(t *testing.T) {
	home := fakeThunderbird(t)
	inbox := filepath.Join(home, ".thunderbird", "abcd.default-release", "ImapMail", "imap.example.com", "INBOX")

	phish := "\nFrom - Thu Oct 09 10:00:00 2026\n" +
		"X-Mozilla-Status: 0001\n" +
		"From: PayPal Service <alert@paypa1-secure.com>\n" +
		"Reply-To: desk@claims-center.net\n" +
		"Subject: Your account will be suspended\n" +
		"Date: Thu, 9 Oct 2026 10:00:00 +0300\n" +
		"\n" +
		"Verify your account within 24 hours: https://paypa1-secure.com/login\n"

	f, err := os.OpenFile(inbox, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	f.WriteString(phish)
	f.Close()

	var look scam.Lookup = fakeLookup{"paypa1-secure.com": false, "example.org": true, "grid.example": true}
	m := ReadMailbox{Home: home, Lookup: func() scam.Lookup { return look }}

	list := mailbox(t, m, `{"action":"list","folder":"ImapMail/imap.example.com/INBOX"}`)
	if !strings.Contains(list, "Your account will be suspended [scam check: likely a scam]") {
		t.Errorf("the list does not flag the phish:\n%s", list)
	}

	if strings.Contains(list, "Planned interruption [scam") {
		t.Errorf("ordinary mail was flagged:\n%s", list)
	}

	read := mailbox(t, m, `{"action":"read","folder":"ImapMail/imap.example.com/INBOX","number":4}`)
	for _, want := range []string{"Reply-To: desk@claims-center.net", "LIKELY A SCAM", "does not exist", "imitates paypal"} {
		if !strings.Contains(read, want) {
			t.Errorf("%q missing from:\n%s", want, read)
		}
	}

	ordinary := mailbox(t, m, `{"action":"read","folder":"ImapMail/imap.example.com/INBOX","number":1}`)
	if !strings.Contains(ordinary, "Scam check: no warning signs.") {
		t.Errorf("an ordinary message:\n%s", ordinary)
	}
}
