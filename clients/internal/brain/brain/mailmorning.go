package brain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The morning mail summary.
 *
 * Once a day, from eight in the morning, when its owner has switched it on:
 * new mail counted the way the Mail page sorts it, and the drafts waiting for
 * a yes — one line in the conversation, said aloud if they are listening.
 * Only numbers and senders' names: nothing is opened, nothing is answered.
 *
 * The day it was last said is kept in the brain's folder, so a restart at ten
 * does not say it a second time.
 */

// MorningHour is when the summary is due, local time.
const MorningHour = 8

func (b *Brain) mailMornings(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(SettleBeforeLooking):
	}

	for {
		if b.Cfg.MailMorning {
			if line := b.mailMorningLine(ctx, time.Now()); line != "" {
				b.sayWhenFinished(line)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Minute):
		}
	}
}

func (b *Brain) mailMorningMark() string { return filepath.Join(b.Root, "mail-morning") }

/*
 * mailMorningLine is the summary if one is due now, and marks the day; empty
 * when it is not due, already said today, or there is no mailbox.
 */
func (b *Brain) mailMorningLine(ctx context.Context, now time.Time) string {
	if now.Hour() < MorningHour {
		return ""
	}

	today := now.Format("2006-01-02")

	if said, err := os.ReadFile(b.mailMorningMark()); err == nil && strings.TrimSpace(string(said)) == today {
		return ""
	}

	d, _, err := b.Desk.Digest(ctx, 24)
	if err != nil {
		return ""
	}

	_ = os.WriteFile(b.mailMorningMark(), []byte(today+"\n"), 0o600)

	waiting := 0

	if b.DB != nil {
		if list, err := b.DB.Drafts(store.DraftWaiting, 100); err == nil {
			waiting = len(list)
		}
	}

	return morningWords(uiLanguageOf(b.Cfg.Language), len(d.Reply), len(d.Suspicious), len(d.People), len(d.Bulk), waiting, d.Total)
}

func uiLanguageOf(spoken string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(spoken)), "bg") {
		return "bg"
	}

	return "en"
}

// morningWords is the summary in the owner's language.
func morningWords(lang string, reply, suspicious, people, bulk, drafts, total int) string {
	if lang == "bg" {
		if total == 0 && drafts == 0 {
			return "Пощата от последното денонощие: нищо ново."
		}

		parts := []string{fmt.Sprintf("Пощата от последното денонощие: %d писма", total)}

		if reply > 0 {
			parts = append(parts, fmt.Sprintf("чакат отговор: %d", reply))
		}

		if people > 0 {
			parts = append(parts, fmt.Sprintf("от хора: %d", people))
		}

		if suspicious > 0 {
			parts = append(parts, fmt.Sprintf("подозрителни: %d — не действайте по тях без проверка", suspicious))
		}

		if bulk > 0 {
			parts = append(parts, fmt.Sprintf("бюлетини и известия: %d", bulk))
		}

		line := strings.Join(parts, "; ") + "."

		if drafts > 0 {
			line += fmt.Sprintf(" Подготвени отговори, които чакат вашето „да“: %d — на страницата „Поща“.", drafts)
		}

		return line
	}

	if total == 0 && drafts == 0 {
		return "Mail from the last day: nothing new."
	}

	parts := []string{fmt.Sprintf("Mail from the last day: %d messages", total)}

	if reply > 0 {
		parts = append(parts, fmt.Sprintf("%d need a reply", reply))
	}

	if people > 0 {
		parts = append(parts, fmt.Sprintf("%d from people", people))
	}

	if suspicious > 0 {
		parts = append(parts, fmt.Sprintf("%d look suspicious — do not act on them without checking", suspicious))
	}

	if bulk > 0 {
		parts = append(parts, fmt.Sprintf("%d newsletters and notifications", bulk))
	}

	line := strings.Join(parts, "; ") + "."

	if drafts > 0 {
		line += fmt.Sprintf(" %d drafted replies wait for your yes on the Mail page.", drafts)
	}

	return line
}
