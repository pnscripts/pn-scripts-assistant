package brain

import (
	"context"
	"fmt"
	"time"

	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/tools"
)

/*
 * Reminders, and the part that makes them worth having: saying them.
 *
 * A reminder nobody is told about is a row in a table. The worker below is what
 * turns it into the brain speaking up, and it is deliberately the only thing in
 * this program that talks without being spoken to first.
 */

// diaryOf adapts the store to what the reminder tools need.
type diaryOf struct{ b *Brain }

func (d diaryOf) AddReminder(what string, at time.Time) (int64, string, error) {
	r, err := d.b.DB.AddReminder(what, at)
	if err != nil {
		return 0, "", err
	}

	return r.ID, at.Local().Format("Mon 2 Jan 15:04"), nil
}

func (d diaryOf) ListReminders(includePast bool) ([]tools.DiaryItem, error) {
	rows, err := d.b.DB.Reminders(includePast, 50)
	if err != nil {
		return nil, err
	}

	out := make([]tools.DiaryItem, 0, len(rows))

	for _, r := range rows {
		out = append(out, tools.DiaryItem{
			ID: r.ID, What: r.What, At: r.At, Said: r.SaidAt != nil,
		})
	}

	return out, nil
}

func (d diaryOf) ForgetReminder(id int64) error { return d.b.DB.ForgetReminder(id) }

// howOftenToCheckReminders is a compromise: often enough that "in a minute"
// means roughly a minute, rare enough to be free on a machine this busy.
const howOftenToCheckReminders = 20 * time.Second

/*
 * watchReminders says what is due, out loud.
 *
 * Marked as said only once it has actually been said, so a brain stopped
 * between the two still owes the reminder rather than having swallowed it. The
 * opposite order loses exactly the reminders that matter most: the ones due
 * when something went wrong.
 */
func (b *Brain) watchReminders(ctx context.Context) {
	ticker := time.NewTicker(howOftenToCheckReminders)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		due, err := b.DB.DueReminders(time.Now())
		if err != nil {
			b.Log.Warn("could not read reminders", "error", err)

			continue
		}

		for _, r := range due {
			if ctx.Err() != nil {
				return
			}

			// Not over the top of a conversation: waiting is better than
			// talking across somebody, and the reminder is still due after.
			if speech.Speaking() || speech.Recording() {
				break
			}

			line := fmt.Sprintf("%s, a reminder: %s", b.Cfg.Owner, r.What)

			if err := speech.SpeakAndWait(ctx, line); err != nil {
				b.Log.Warn("could not say a reminder", "id", r.ID, "error", err)

				continue
			}

			if err := b.DB.MarkReminderSaid(r.ID, time.Now()); err != nil {
				b.Log.Warn("said a reminder but could not record it", "id", r.ID, "error", err)
			}
		}
	}
}
