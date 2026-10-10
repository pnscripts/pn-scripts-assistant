package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
