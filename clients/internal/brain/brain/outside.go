package brain

import (
	"errors"
	"fmt"
	"time"

	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * What other programs may ask (package otherprograms), as the brain keeps it:
 * the switches its person sets at the desk, and the record of every
 * question another program put through the MCP server, with the name that
 * program gave.
 */

// OutsideMay reports whether other programs may ask about subject. A
// switch that cannot be read is off: a safety switch that cannot do its
// check refuses, it does not pass.
func (b *Brain) OutsideMay(subject otherprograms.Subject) bool {
	if !otherprograms.Known(subject) {
		return false
	}

	may, err := b.DB.OutsideAllowed()
	if err != nil {
		b.Log.Warn("could not read what other programs may ask; nothing is allowed", "err", err)

		return false
	}

	_, allowed := may[string(subject)]

	return allowed
}

/*
 * AllowOutside sets one switch: whether other programs may ask about
 * subject. A change is written on the record of what was done, whoever
 * made it: the assistant's own page and a program on this computer that
 * posts to the same address cannot be told apart, so the record does not
 * say "you", it says where. The change and its record are one thing: a
 * change that cannot be recorded is not made (store.SwitchOutside). A
 * switch set to what it already is changes nothing and records nothing.
 */
func (b *Brain) AllowOutside(subject otherprograms.Subject, allowed bool) error {
	if !otherprograms.Known(subject) {
		return fmt.Errorf("%q is not something other programs can be allowed to ask about", subject)
	}

	en, bg := otherprograms.Switched(subject, allowed)

	// Who made the change is not said: the assistant's own page and a
	// program posting to the same address look the same. The page says
	// where instead (deeds.js, for this tool).
	changed, err := b.DB.SwitchOutside(string(subject), allowed, time.Now(), store.Deed{
		Tool:      SwitchTool,
		Summary:   en,
		SummaryBG: bg,
		Outcome:   "done",
		Changes:   true,
	})
	if err != nil {
		return err
	}

	if changed {
		b.Log.Info("what other programs may ask was changed", "about", string(subject), "allowed", allowed)
	}

	return nil
}

// SwitchTool is the name the record keeps a change of a switch under.
const SwitchTool = "other_programs_switch"

// OutsideSwitch is one switch as the page shows it.
type OutsideSwitch struct {
	ID      otherprograms.Subject `json:"id"`
	Allowed bool                  `json:"allowed"`

	// Since is when it was allowed; nil while it is not.
	Since *time.Time `json:"since"`
}

// OutsideAsked is one question another program asked, as the page lists
// the last few beside the switches.
type OutsideAsked struct {
	By      string    `json:"by"`
	Tool    string    `json:"tool"`
	Outcome string    `json:"outcome"`
	At      time.Time `json:"at"`
}

// OutsideRecent is how many of the last questions the page is given.
const OutsideRecent = 5

/*
 * OutsideSwitches is every switch, in the page's order, and the last few
 * questions other programs asked. Unlike OutsideMay it fails when the
 * switches cannot be read: the page says so instead of showing "off".
 */
func (b *Brain) OutsideSwitches() ([]OutsideSwitch, []OutsideAsked, error) {
	may, err := b.DB.OutsideAllowed()
	if err != nil {
		return nil, nil, err
	}

	switches := make([]OutsideSwitch, 0, len(otherprograms.Subjects()))

	for _, s := range otherprograms.Subjects() {
		one := OutsideSwitch{ID: s}

		if since, allowed := may[string(s)]; allowed {
			one.Allowed, one.Since = true, &since
		}

		switches = append(switches, one)
	}

	deeds, err := b.DB.Deeds(store.DeedFilter{Outside: true, Limit: OutsideRecent})
	if err != nil {
		return nil, nil, err
	}

	recent := make([]OutsideAsked, 0, len(deeds))
	for _, d := range deeds {
		recent = append(recent, OutsideAsked{By: d.AskedBy, Tool: d.Tool, Outcome: d.Outcome, At: d.CreatedAt})
	}

	return switches, recent, nil
}

/*
 * AskedFromOutside writes down one question another program asked: which,
 * who (otherprograms.Caller), what it was given (asked, a short line), how it
 * ended ("done" or "refused") and what was said of that. It fails when
 * the record cannot be written, and the caller then does not answer: what
 * another program was told is always on the record.
 */
func (b *Brain) AskedFromOutside(tool, by, asked, outcome, result string) error {
	en, bg := otherprograms.Asked(tool)
	if en == "" {
		return fmt.Errorf("%q is not a question other programs may ask", tool)
	}

	if by == "" {
		return errors.New("a question from another program with nobody named")
	}

	_, err := b.DB.AddDeed(store.Deed{
		Tool:      tool,
		Summary:   en,
		SummaryBG: bg,
		Asked:     asked,
		Result:    result,
		Outcome:   outcome,
		AskedBy:   by,
	})

	return err
}
