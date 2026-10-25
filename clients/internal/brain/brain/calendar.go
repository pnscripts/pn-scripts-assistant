package brain

import (
	"pn-scripts-assistant/internal/brain/caldav"
	"pn-scripts-assistant/internal/brain/tools"
)

// planner is where the planning tools look: the diary, the reminders, the
// to-do lists, and the online calendar as it is set up now.
func (b *Brain) planner() tools.Planner {
	return tools.Planner{
		Diary:     b.DB,
		Reminders: diaryOf{b},
		Todos:     b.DB,
		Online:    b.OnlineCalendar,
	}
}

// Planner is the same for the pages.
func (b *Brain) Planner() tools.Planner { return b.planner() }

// OnlineCalendar is the owner's online calendar as configured now.
func (b *Brain) OnlineCalendar() caldav.Calendar {
	return caldav.Calendar{URL: b.Cfg.CalendarURL, User: b.Cfg.CalendarUser, Password: b.Cfg.CalendarPassword}
}

func (b *Brain) calendarTools() []tools.Tool {
	return []tools.Tool{tools.ReadCalendar{Plan: b.planner()}, tools.WriteCalendar{Plan: b.planner()}}
}

/*
 * CalendarSetUp registers the online calendar's tools once one has been
 * set up, and takes them away again when it is removed — so the model is
 * offered them exactly when they can work, without a restart.
 */
func (b *Brain) CalendarSetUp(configured bool) {
	if b.Agent == nil || b.Agent.Registry == nil {
		return
	}

	for _, t := range b.calendarTools() {
		if configured {
			if _, there := b.Agent.Registry.Get(t.Name()); !there {
				_ = b.Agent.Registry.Register(t)
			}

			continue
		}

		b.Agent.Registry.Unregister(t.Name())
	}
}
