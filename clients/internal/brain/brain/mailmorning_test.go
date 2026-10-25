package brain

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mail"
	"pn-scripts-assistant/internal/brain/tools"
)

func TestTheMorningSummaryIsInTheOwnersLanguage(t *testing.T) {
	en := morningWords("en", 3, 1, 2, 12, 2, 18)
	if !strings.Contains(en, "18 messages; 3 need a reply; 2 from people; 1 look suspicious") || !strings.Contains(en, "2 drafted replies") {
		t.Errorf("%s", en)
	}

	bg := morningWords("bg", 0, 0, 0, 0, 0, 0)
	if bg != "Пощата от последното денонощие: нищо ново." {
		t.Errorf("%s", bg)
	}

	if !strings.Contains(morningWords("bg", 1, 0, 0, 0, 1, 1), "„Поща“") {
		t.Error("the Bulgarian summary does not say where the drafts are")
	}
}

// Not before eight, not without a mailbox, and never twice in a day.
func TestTheMorningSummaryIsSaidOnceADay(t *testing.T) {
	b := quietBrain(t)
	b.Desk = tools.MailDesk{Account: func() mail.Account { return mail.Account{} }, Local: tools.ReadMailbox{Home: t.TempDir()}}

	early := time.Date(2026, 10, 26, 7, 0, 0, 0, time.Local)
	if line := b.mailMorningLine(context.Background(), early); line != "" {
		t.Errorf("said before eight: %s", line)
	}

	if line := b.mailMorningLine(context.Background(), early.Add(2*time.Hour)); line != "" {
		t.Errorf("said with no mailbox: %s", line)
	}
}
