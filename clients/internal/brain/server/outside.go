package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The door other programs ask through, and the switch at the desk for
 * what they may ask (package otherprograms).
 *
 * "pn-scripts-assistant mcp" puts every question of another program
 * through POST /api/outside/ask, with the name that program gave. One
 * door, so that three things happen in one place and cannot be forgotten
 * on a route: the switch is checked, the question is written on the
 * record with who asked, and only then is it answered, with exactly what
 * that question is for and nothing more.
 *
 * A POST, because asking writes the record. Like every route that changes
 * something it is refused to a web page on another site (Guard), so a page
 * open in a browser cannot ask as "another program"; and it is under
 * /api/outside, which a paired device with limited rights may not post to.
 */

// mostAsked is far more than a question to these tools needs.
const mostAsked = 16 << 10

// strictly reads one JSON object into into and nothing after it, refusing
// a field it does not know.
func strictly(w http.ResponseWriter, r *http.Request, into any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mostAsked))
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(into); err != nil {
		return err
	}

	if dec.More() {
		return errors.New("more than one request")
	}

	return nil
}

/*
 * fromAPage reports whether a browser sent the request from a page that
 * is not the assistant's own. Guard already refuses another site by its
 * Origin; a browser also says where a request began in Sec-Fetch-Site,
 * and here both are read, because the two things behind this file are
 * the ones a page must never do: speak as "another program", and decide
 * what other programs may ask.
 */
func fromAPage(r *http.Request, ownPageToo bool) bool {
	site := r.Header.Get("Sec-Fetch-Site")

	if ownPageToo {
		return site != "" || r.Header.Get("Origin") != ""
	}

	return site != "" && site != "same-origin" && site != "none"
}

// toldProgram is a program as another program is told of it: its name,
// version, source and whether it is here. Never where it is or how it is
// started.
type toldProgram struct {
	Kind      programs.Kind `json:"kind"`
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Version   string        `json:"version,omitempty"`
	Present   bool          `json:"present"`
	RemovedAt *time.Time    `json:"removed_at,omitempty"`
}

func (s *Server) handleOutsideAsk(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Client    struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"client"`
	}

	// A program says nothing about pages; anything that does is a browser,
	// and no page asks as another program, not even the assistant's own.
	if fromAPage(r, true) {
		fail(w, http.StatusForbidden, "Only a program on this computer asks here, not a page.")

		return
	}

	if err := strictly(w, r, &asked); err != nil {
		fail(w, http.StatusBadRequest, "That request could not be read.")

		return
	}

	subject, known := otherprograms.SubjectOf(asked.Tool)
	if !known {
		fail(w, http.StatusBadRequest, "That is not a question other programs may ask.")

		return
	}

	by := otherprograms.Caller(asked.Client.Name, asked.Client.Version)

	// What the question was given, read before anything is decided: a
	// question that cannot be read is not a question, and is not recorded.
	given, problem := outsideGiven(asked.Tool, asked.Arguments)
	if problem != "" {
		fail(w, http.StatusBadRequest, problem)

		return
	}

	/*
	 * However much is allowed, only so many questions a minute, from all
	 * programs together. The first one over is on the record. Fetching the
	 * answer to a question already taken is not another question: it does
	 * no work and writes nothing, and counting it would let one program's
	 * flood lose another program its answer.
	 */
	if given.answerTo != "" {
		// Not counted.
	} else if within, first := s.outsidePace.Next(time.Now()); !within {
		if first {
			en, _ := otherprograms.TooMany()

			if err := s.brain.AskedFromOutside(asked.Tool, by, "", agent.DeedRefused, en); err != nil {
				s.log.Warn("could not record that other programs asked too much", "tool", asked.Tool, "error", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Other programs have asked too many questions in the last minute.",
			"outside": "too many",
		})

		return
	}

	// notAllowed refuses a question whose switch is off, and writes the
	// refusal on the record.
	notAllowed := func() {
		/*
		 * A refusal that cannot be written down is still a refusal. What
		 * was asked is not kept with it: nothing was done with it. And a
		 * fetch is not written at all: it is not counted by the minute's
		 * number (above), so a line for each would let a program that
		 * fetches in a loop fill the record, and its question, if there
		 * was one, has its own line already.
		 */
		if given.answerTo == "" {
			en, _ := otherprograms.NotAllowed(subject)

			if err := s.brain.AskedFromOutside(asked.Tool, by, given.kindLine(), agent.DeedRefused, en); err != nil {
				s.log.Warn("could not record a question another program asked", "tool", asked.Tool, "error", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Its user has not allowed other programs to ask that.",
			"outside": "not allowed",
			"subject": string(subject),
		})
	}

	if subject != "" && !s.brain.OutsideMay(subject) {
		notAllowed()

		return
	}

	// No record, no answer: what another program was told is always
	// written down first.
	noted := func(tool, askedFor, result string) bool {
		if err := s.brain.AskedFromOutside(tool, by, askedFor, agent.DeedDone, result); err != nil {
			s.log.Warn("could not record a question another program asked; not answering it", "tool", tool, "error", err)
			fail(w, http.StatusInternalServerError, "The question could not be written on the record, so it was not answered.")

			return false
		}

		return true
	}

	switch asked.Tool {
	case "assistant_status":
		if noted(asked.Tool, "", "") {
			ok(w, map[string]any{"noted": true})
		}

	case "list_programs":
		records, _, err := s.brain.ProgramInventory()
		if err != nil {
			fail(w, http.StatusInternalServerError, "The list of programs could not be read.")

			return
		}

		told := make([]toldProgram, 0, len(records))

		for _, rec := range records {
			if given.kind == "" || rec.Kind == given.kind {
				told = append(told, toldProgram{Kind: rec.Kind, ID: rec.ID, Name: rec.Name, Version: toldVersion(rec.Version),
					Present: rec.Present, RemovedAt: rec.RemovedAt})
			}
		}

		if noted(asked.Tool, given.kindLine(), fmt.Sprintf("Programs I told it about: %d.", len(told))) {
			ok(w, map[string]any{"programs": told})
		}

	case "search_memory":
		found, err := s.brain.SearchForOutside(r.Context(), given.search, given.limit)

		switch {
		case errors.Is(err, brain.ErrNoMemorySearch):
			s.log.Warn("the memory could not be searched for another program", "error", err)
			fail(w, http.StatusServiceUnavailable, "The memory cannot be searched now: the assistant has no model on this computer to search with.")

			return
		case err != nil:
			s.log.Warn("the memory could not be read for another program", "error", err)
			fail(w, http.StatusInternalServerError, "The memory could not be read.")

			return
		}

		// The label keeps another program's words from standing at the
		// start of what is stored.
		if noted(asked.Tool, "Search: "+given.search, fmt.Sprintf("Things I remember that I told it: %d.", len(found))) {
			ok(w, map[string]any{"found": found})
		}

	case "ask_assistant":
		s.outsideAnswer(w, r, by, given, notAllowed)

	case otherprograms.PermissionTool:
		s.outsidePermit(w, r, by, given, notAllowed)
	}
}

/*
 * permitWait is how long the door holds a permission question of another
 * program open for its person's answer. Claude Code waits half an hour for
 * a tool that says nothing, so this is well under it; a question nobody
 * answered by then is answered no. A variable so a test does not wait.
 */
var permitWait = permitWaitDefault

// permitWaitDefault is what the page and the guides say of it ("ten
// minutes"); a test keeps them together.
const permitWaitDefault = 10 * time.Minute

// notAPermission is what Claude Code calls the questions it wants to put
// to its user itself: they need the user's words back, which this door
// does not carry.
const notAPermission = "AskUserQuestion"

/*
 * outsidePermit puts a permission question of another program to the
 * assistant's person and answers with what they said. Every ending but
 * their own yes is "deny", with how it ended, so the program can say why.
 */
func (s *Server) outsidePermit(w http.ResponseWriter, r *http.Request, by string, given outsideAsked, notAllowed func()) {
	denied := func(ended string) {
		ok(w, map[string]any{"behavior": "deny", "ended": ended})
	}

	if given.permitTool == notAPermission {
		const why = "Not answered: I pass on a program's permission questions only, not the questions it wants to ask you itself."

		if err := s.brain.AskedFromOutside(otherprograms.PermissionTool, by, "", agent.DeedRefused, why); err != nil {
			s.log.Warn("could not record a question another program asked", "tool", otherprograms.PermissionTool, "error", err)
		}

		denied("not a permission")

		return
	}

	// The person may take minutes: the connection is given the time.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(permitWait + time.Minute))

	allowed, ended, err := s.brain.AskPermissionForOutside(r.Context(), by, given.permitTool, given.permitInput, given.permitCut, permitWait)

	switch {
	case errors.Is(err, brain.ErrOutsideNotAllowed):
		// Switched off between the door's own look and the brain's.
		notAllowed()
	case errors.Is(err, brain.ErrPermitsFull):
		// On the record like every refusal; the minute's number bounds
		// how many such lines a program can write.
		full := fmt.Sprintf("Not answered: %d permission questions were waiting already, and no more wait at once.", otherprograms.MostPermissionsWaiting)

		if err := s.brain.AskedFromOutside(otherprograms.PermissionTool, by, "", agent.DeedRefused, full); err != nil {
			s.log.Warn("could not record a question another program asked", "tool", otherprograms.PermissionTool, "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Too many permission questions are waiting for the assistant's user already.",
			"outside": "permits full",
		})
	case errors.Is(err, brain.ErrNoPermission):
		fail(w, http.StatusBadRequest, "tool_name is what permission is asked for: it had nothing readable in it.")
	case err != nil:
		s.log.Warn("a permission question of another program could not be put to its user", "error", err)
		fail(w, http.StatusInternalServerError, "The question could not be put to the assistant's user.")
	case allowed:
		ok(w, map[string]any{"behavior": "allow"})
	default:
		denied(string(ended))
	}
}

// outsideAsked is what a question was given, read and bounded.
type outsideAsked struct {
	kind     programs.Kind
	search   string
	limit    int
	question string
	answerTo string

	// A permission question: what it is about, and what would be given.
	permitTool  string
	permitInput json.RawMessage

	// How long each text of the input really is, where the MCP server
	// sent it cut.
	permitCut map[string]int
}

// kindLine is the source asked for, as the record keeps it.
func (g outsideAsked) kindLine() string {
	if g.kind == "" {
		return ""
	}

	return "kind: " + string(g.kind)
}

/*
 * outsideGiven reads the arguments of one question. Each question takes
 * its own and no others; what is text is made safe to keep and bounded
 * (otherprograms.Words). The second value says what is wrong, for the
 * program that asked.
 */
func outsideGiven(tool string, raw json.RawMessage) (outsideAsked, string) {
	const notThose = "That question does not take those arguments."

	var (
		given     outsideAsked
		arguments struct {
			Kind     *string `json:"kind"`
			Query    *string `json:"query"`
			Limit    *int    `json:"limit"`
			Question *string `json:"question"`
			AnswerTo *string `json:"answer_to"`

			// A permission question, as Claude Code hands it to its
			// permission tool.
			ToolName  *string         `json:"tool_name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID *string         `json:"tool_use_id"`
			Cut       json.RawMessage `json:"cut"`
		}
	)

	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&arguments); err != nil || dec.More() {
			return given, notThose
		}
	}

	if (arguments.Kind != nil && tool != "list_programs") ||
		((arguments.Query != nil || arguments.Limit != nil) && tool != "search_memory") ||
		((arguments.Question != nil || arguments.AnswerTo != nil) && tool != "ask_assistant") ||
		((arguments.ToolName != nil || arguments.Input != nil || arguments.ToolUseID != nil || arguments.Cut != nil) && tool != otherprograms.PermissionTool) {
		return given, notThose
	}

	switch tool {
	case "list_programs":
		if arguments.Kind != nil {
			if given.kind = programs.Kind(*arguments.Kind); given.kind != "" && !programs.Known(given.kind) {
				return given, "kind is app, deb, snap, flatpak, win or msix"
			}
		}

	case "search_memory":
		if arguments.Query == nil || len([]rune(*arguments.Query)) > otherprograms.MostSearch {
			return given, fmt.Sprintf("query is what to search for, at most %d characters.", otherprograms.MostSearch)
		}

		if given.search = otherprograms.Words(*arguments.Query, otherprograms.MostSearch); given.search == "" {
			return given, "query is what to search for: it had nothing readable in it."
		}

		given.limit = brain.OutsideFactsDefault

		if arguments.Limit != nil {
			if given.limit = *arguments.Limit; given.limit < 1 || given.limit > brain.OutsideFacts {
				return given, fmt.Sprintf("limit is from 1 to %d.", brain.OutsideFacts)
			}
		}

	case otherprograms.PermissionTool:
		const takes = "tool_name is what permission is asked for, input is what it would be given (an object)."

		if arguments.ToolName == nil || len(*arguments.ToolName) > 4*otherprograms.MostPermissionTool {
			return given, takes
		}

		if given.permitTool = otherprograms.PermissionAsks(*arguments.ToolName); given.permitTool == "" {
			return given, "tool_name is what permission is asked for: it had nothing readable in it."
		}

		if raw := bytes.TrimSpace(arguments.Input); len(raw) > 0 && string(raw) != "null" {
			if raw[0] != '{' || !json.Valid(raw) {
				return given, takes
			}

			given.permitInput = json.RawMessage(raw)
		}

		// A few lengths, each of a text that long: anything else is not
		// a length.
		if raw := bytes.TrimSpace(arguments.Cut); len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, &given.permitCut) != nil || len(given.permitCut) > 64 {
				return given, takes
			}

			for name, length := range given.permitCut {
				if length < 1 || length > 1<<30 || len(name) > 200 {
					return given, takes
				}
			}
		}

	case "ask_assistant":
		// A question, or the ticket of one asked before: one of the two.
		if (arguments.Question == nil) == (arguments.AnswerTo == nil) {
			return given, "Give question (what to ask), or answer_to (the ticket of a question already asked), not both."
		}

		if arguments.AnswerTo != nil {
			if given.answerTo = *arguments.AnswerTo; !ticketLike(given.answerTo) {
				return given, "answer_to is the ticket a question was given, as it was given."
			}

			break
		}

		if len([]rune(*arguments.Question)) > otherprograms.MostQuestion {
			return given, fmt.Sprintf("question is at most %d characters.", otherprograms.MostQuestion)
		}

		if given.question = otherprograms.Words(*arguments.Question, otherprograms.MostQuestion); given.question == "" {
			return given, "question is what to ask: it had nothing readable in it."
		}
	}

	return given, ""
}

// ticketLike reports whether s has the shape of a ticket: 32 hex digits.
func ticketLike(s string) bool {
	if len(s) != 32 {
		return false
	}

	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

/*
 * outsideWait is how long the door holds a question to the assistant open
 * for its answer. The programs that ask give up on a call after about a
 * minute (the Model Context Protocol's own client: 60 seconds), and the
 * model here can take longer, so the door answers within this: with the
 * answer, or with the ticket to fetch it with. A variable so a test does
 * not wait.
 */
var outsideWait = 40 * time.Second

/*
 * outsideAnswer takes a question to the assistant itself, or fetches the
 * answer to one taken before. The question is written on the record, with
 * who asked, when its answer is ready and before anybody is given it
 * (brain.AskForOutside).
 */
func (s *Server) outsideAnswer(w http.ResponseWriter, r *http.Request, by string, given outsideAsked, notAllowed func()) {
	ticket := given.answerTo

	if ticket == "" {
		var err error

		ticket, err = s.brain.AskForOutside(by, given.question)

		switch {
		case errors.Is(err, brain.ErrOutsideNotAllowed):
			// Switched off between the door's own look and the brain's.
			notAllowed()

			return
		case errors.Is(err, brain.ErrOutsideBusy):
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "The assistant is answering another question now.",
				"outside": "busy",
			})

			return
		case errors.Is(err, brain.ErrNoLocalModel):
			fail(w, http.StatusServiceUnavailable, "The assistant has no model on this computer to answer with, and other programs are answered by no other.")

			return
		case err != nil:
			s.log.Warn("a question from another program could not be taken", "error", err)
			fail(w, http.StatusInternalServerError, "The question could not be taken.")

			return
		}
	}

	letItThink(w)

	answer, ready, err := s.brain.OutsideAnswer(r.Context(), ticket, outsideWait)

	switch {
	case errors.Is(err, brain.ErrOutsideGaveWay):
		// On the record already, as a question given up for its user's own.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "The assistant stopped to answer its user, who comes first. Ask the question again in a moment.",
			"outside": "gave way",
		})
	case errors.Is(err, brain.ErrNoSuchAnswer):
		// Not 404: the MCP server reads that as an assistant with no door.
		fail(w, http.StatusGone, "No answer is waiting under that ticket: it was fetched too late, a newer question took its place, or its user took the permission back. Ask the question again.")
	case err != nil:
		// The brain's own words for it; none of them carries the model's.
		fail(w, http.StatusBadGateway, "Not answered: "+err.Error()+".")
	case !ready:
		ok(w, map[string]any{"ready": false, "answer_to": ticket})
	default:
		ok(w, map[string]any{"ready": true, "answer": answer})
	}
}

// toldVersion is a version as another program is told it: a menu entry's
// fingerprint is not one.
func toldVersion(v string) string {
	if programs.IsPin(v) {
		return ""
	}

	return v
}

// outsideState is the switches and the last questions, as the page reads
// them.
func (s *Server) outsideState(w http.ResponseWriter) {
	switches, recent, err := s.brain.OutsideSwitches()
	if err != nil {
		fail(w, http.StatusInternalServerError, "What other programs may ask could not be read.")

		return
	}

	ok(w, map[string]any{"subjects": switches, "recent": recent})
}

func (s *Server) handleOutside(w http.ResponseWriter, _ *http.Request) { s.outsideState(w) }

// handleOutsideAllow sets one switch, at the desk only.
func (s *Server) handleOutsideAllow(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Subject string `json:"subject"`
		Allowed *bool  `json:"allowed"`
	}

	if fromAPage(r, false) {
		fail(w, http.StatusForbidden, "that request came from somewhere else")

		return
	}

	if err := strictly(w, r, &asked); err != nil || asked.Allowed == nil || !otherprograms.Known(otherprograms.Subject(asked.Subject)) {
		fail(w, http.StatusBadRequest, "Say which (programs, ask, memory or permissions) and whether it is allowed (true or false).")

		return
	}

	if err := s.brain.AllowOutside(otherprograms.Subject(asked.Subject), *asked.Allowed); err != nil {
		fail(w, http.StatusInternalServerError, "That could not be kept.")

		return
	}

	s.outsideState(w)
}
