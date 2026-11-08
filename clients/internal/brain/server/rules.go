package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/polish"
	"pn-scripts-assistant/internal/brain/rules"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The Rules page: what its user has told the assistant always or never to
 * do. Proposals noticed in conversation wait at the top for keep, edit or
 * decline; kept rules have a switch, can be edited and deleted, and say
 * where they came from and what each one does to an action (rules.Mode).
 * Changing any of it is a decision, so it is made at the computer (see
 * changedAtTheDeskOnly).
 */

type ruleView struct {
	rules.Rule
	Mode  string      `json:"mode"`
	Scope rules.Scope `json:"scope"`
}

func viewOf(r rules.Rule) ruleView {
	return ruleView{Rule: r, Mode: rules.Mode(r), Scope: rules.ScopeOf(r)}
}

func (s *Server) handleRules(w http.ResponseWriter, _ *http.Request) {
	list, err := s.brain.DB.Rules(false)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	views := make([]ruleView, 0, len(list))
	for _, r := range list {
		views = append(views, viewOf(r))
	}

	ok(w, map[string]any{"rules": views, "kinds": rules.Kinds(), "actions": rules.Actions(), "most": store.MaxRules})
}

type ruleBody struct {
	Text    *string   `json:"text"`
	Kind    *string   `json:"kind"`
	Actions *[]string `json:"actions"`
	Words   *[]string `json:"words"`
	Days    *[]int    `json:"days"`
}

// into puts what was sent over r. A field not sent is left as it was; a
// rule's kinds of action, when not sent for a new one, are read from its
// words.
func (b ruleBody) into(r rules.Rule) (rules.Rule, error) {
	if b.Text != nil {
		r.Text = *b.Text
	}

	if strings.TrimSpace(r.Text) == "" {
		return r, errors.New("A rule needs words.")
	}

	if b.Kind != nil {
		if !rules.ValidKind(rules.Kind(*b.Kind)) {
			return r, errors.New("That is not a kind of rule.")
		}

		r.Kind = rules.Kind(*b.Kind)
	}

	if b.Actions != nil {
		for _, a := range *b.Actions {
			if !rules.ValidAction(a) {
				return r, errors.New("That is not a kind of action: " + a)
			}
		}

		r.Actions = *b.Actions
	}

	if b.Words != nil {
		if len(*b.Words) > rules.MaxWords {
			return r, errors.New("A rule can name at most " + strconv.Itoa(rules.MaxWords) + " words.")
		}

		r.Words = *b.Words
	}

	if b.Days != nil {
		r.Days = nil

		for _, d := range *b.Days {
			if d < 0 || d > 6 {
				return r, errors.New("That is not a day of the week.")
			}

			r.Days = append(r.Days, time.Weekday(d))
		}
	}

	return r, nil
}

// handleRuleAdd keeps a rule written on the Rules page. Written there by its
// user, it is theirs already: kept and on, with nothing to propose.
func (s *Server) handleRuleAdd(w http.ResponseWriter, r *http.Request) {
	var body ruleBody

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	rule, err := body.into(rules.Rule{})
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if body.Actions == nil {
		rule.Actions = rules.ActionsIn(rule.Text)
	}

	if body.Words == nil {
		rule.Words = rules.SpecificWords(rule.Text)
	}

	if body.Days == nil {
		rule.Days = rules.DaysIn(rule.Text)
	}

	// No source and no conversation: the page says it was written here.
	rule.Status, rule.On, rule.Source = rules.Kept, true, ""

	kept, err := s.brain.DB.AddRule(rule)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, viewOf(kept))
}

func (s *Server) ruleOf(w http.ResponseWriter, r *http.Request) (rules.Rule, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "That is not a rule's number.")

		return rules.Rule{}, false
	}

	rule, err := s.brain.DB.Rule(id)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, store.ErrNoRule) {
			code = http.StatusNotFound
		}

		fail(w, code, err.Error())

		return rules.Rule{}, false
	}

	return rule, true
}

func (s *Server) handleRuleChange(w http.ResponseWriter, r *http.Request) {
	rule, found := s.ruleOf(w, r)
	if !found {
		return
	}

	var body ruleBody

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	rule, err := body.into(rule)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	changed, err := s.brain.DB.ChangeRule(rule)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, viewOf(changed))
}

/*
 * handleRuleDecide is its user's answer to a proposal: keep it — with any
 * words they changed first — or decline it. The edit is written before the
 * decision, so what is kept is exactly what they were looking at.
 */
func (s *Server) handleRuleDecide(w http.ResponseWriter, r *http.Request) {
	rule, found := s.ruleOf(w, r)
	if !found {
		return
	}

	var body struct {
		Keep bool `json:"keep"`
		ruleBody
	}

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	if rule.Status != rules.Proposed {
		fail(w, http.StatusConflict, "That rule was already "+rule.Status+".")

		return
	}

	if body.Keep && (body.Text != nil || body.Kind != nil || body.Actions != nil || body.Words != nil || body.Days != nil) {
		edited, err := body.into(rule)
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())

			return
		}

		if _, err := s.brain.DB.ChangeRule(edited); err != nil {
			fail(w, http.StatusBadRequest, err.Error())

			return
		}
	}

	decided, err := s.brain.DB.DecideRule(rule.ID, body.Keep)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())

		return
	}

	ok(w, viewOf(decided))
}

func (s *Server) handleRuleSwitch(w http.ResponseWriter, r *http.Request) {
	rule, found := s.ruleOf(w, r)
	if !found {
		return
	}

	var body struct {
		On bool `json:"on"`
	}

	if !decodeUpTo(w, r, &body, 1<<10) {
		return
	}

	switched, err := s.brain.DB.SwitchRule(rule.ID, body.On)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())

		return
	}

	ok(w, viewOf(switched))
}

func (s *Server) handleRuleRemove(w http.ResponseWriter, r *http.Request) {
	rule, found := s.ruleOf(w, r)
	if !found {
		return
	}

	if err := s.brain.DB.RemoveRule(rule.ID); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"removed": rule.ID})
}

// handlePolish is the last look at a text from the Writing page: the text
// as it should go, and what changed or is left to fill in.
func (s *Server) handlePolish(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text    string `json:"text"`
		From    string `json:"from"`
		Subject string `json:"subject"`
		Lang    string `json:"lang"`
	}

	if !decodeUpTo(w, r, &body, 1<<20) {
		return
	}

	// The reader sees the text as coming from the person using the
	// assistant, so "This is <their name> writing" is what they already see.
	from := strings.TrimSpace(body.From)
	if from == "" {
		from = s.brain.Cfg.Owner
	}

	ok(w, polish.Polish(polish.Draft{Text: body.Text, From: from, Subject: body.Subject, Lang: body.Lang}, time.Now()))
}
