package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

/*
 * Changing what has just happened.
 *
 * A conversation you cannot correct is a transcript, not a conversation. The
 * brain could be stopped mid-answer and could be asked something new, and that
 * was the whole of it — a question asked wrongly, a sentence the microphone
 * mangled, something said that should not be in the record, all of it stayed
 * exactly as it landed.
 *
 * These are the four things people actually say when they want the last minute
 * undone: say that again, forget that, throw this away, call it something else.
 * Spoken rather than clicked, and therefore reached through a tool rather than
 * through a button — the same reason everything else here is a tool. Somebody
 * saying "не, забрави това" is asking for exactly the same thing as somebody
 * saying "no, forget that", and a list of English phrases would serve one of
 * them.
 */
type Revise struct {
	// Talk is the part of the store this needs, kept narrow so the tool cannot
	// reach anything it has no business reaching.
	Talk Conversations

	// Now is the conversation this turn belongs to, filled in per turn.
	Now func() int64
}

// Conversations is what revising a conversation needs of the store.
type Conversations interface {
	ForgetLastExchange(conversationID int64) (int, error)
	DeleteConversation(id int64) error
	RenameConversation(id int64, title string) error
	History(conversationID int64) ([]Message, error)
}

// Message is one line of a conversation, as much of it as this needs.
type Message struct {
	Role    string
	Content string
}

func (Revise) Name() string { return "change_this_conversation" }

func (Revise) Description() string {
	return "Act on the conversation itself when asked to undo, repeat or throw away what " +
		"was just said. Use it for \"say that again\", \"answer it properly this time\", " +
		"\"forget that\", \"remove what I just said\", \"delete this conversation\", or " +
		"\"call this something else\". This is for the record of the conversation, not " +
		"for files or anything else on the machine."
}

func (Revise) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"action":{
				"type":"string",
				"enum":["answer_again","forget_the_last_exchange","delete_this_conversation","rename_this_conversation"],
				"description":"answer_again brings back the previous question so it can be answered properly. forget_the_last_exchange removes the last thing said and the answer to it. delete_this_conversation throws the whole thing away. rename_this_conversation gives it a different title."
			},
			"title":{"type":"string","description":"The new title, for rename_this_conversation."}
		},
		"required":["action"],
		"additionalProperties":false
	}`)
}

/*
 * Mutating, so the destructive ones stop and ask.
 *
 * Two of these delete things a person said, and a mis-heard "forget that" would
 * otherwise take a real exchange with it. The one that changes nothing —
 * answering again — is separated out below rather than being made to wait
 * behind an approval it does not need.
 */
func (Revise) Risk() Risk { return Mutating }

func (r Revise) Summarize(raw json.RawMessage) string {
	var a struct {
		Action string `json:"action"`
		Title  string `json:"title"`
	}

	json.Unmarshal(raw, &a)

	switch a.Action {
	case "forget_the_last_exchange":
		return "Forget the last thing said and the answer to it"

	case "delete_this_conversation":
		return "Delete this whole conversation"

	case "rename_this_conversation":
		return "Call this conversation " + strconv.Quote(a.Title)
	}

	return "Answer the previous question again"
}

func (r Revise) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Action string `json:"action"`
		Title  string `json:"title"`
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read the arguments: %w", err)
	}

	if r.Talk == nil || r.Now == nil {
		return "", fmt.Errorf("there is no conversation to change")
	}

	id := r.Now()

	if id == 0 {
		return "", fmt.Errorf("this is not part of a conversation that can be changed")
	}

	switch a.Action {
	case "answer_again":
		return r.previousQuestion(id)

	case "forget_the_last_exchange":
		removed, err := r.Talk.ForgetLastExchange(id)
		if err != nil {
			return "", err
		}

		if removed == 0 {
			return "There was nothing in this conversation to forget.", nil
		}

		return "Forgotten — the last thing said and the answer to it are out of the record.", nil

	case "delete_this_conversation":
		if err := r.Talk.DeleteConversation(id); err != nil {
			return "", err
		}

		return "This conversation is deleted. Nothing it taught me is affected — " +
			"what I learned from it is knowledge and stays.", nil

	case "rename_this_conversation":
		title := strings.TrimSpace(a.Title)

		if title == "" {
			return "", fmt.Errorf("what should it be called")
		}

		if err := r.Talk.RenameConversation(id, title); err != nil {
			return "", err
		}

		return "Renamed to " + strconv.Quote(title) + ".", nil
	}

	return "", fmt.Errorf("%q is not something I can do to a conversation", a.Action)
}

/*
 * previousQuestion finds what was asked before this, so it can be answered
 * again.
 *
 * The one before last, not the last: the last thing said is the request to try
 * again, and answering that would be answering "say that again" rather than
 * the thing it refers to.
 */
func (r Revise) previousQuestion(id int64) (string, error) {
	history, err := r.Talk.History(id)
	if err != nil {
		return "", err
	}

	var asked []string

	for _, m := range history {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			asked = append(asked, m.Content)
		}
	}

	if len(asked) < 2 {
		return "There is no earlier question here to answer again.", nil
	}

	return "The question before this one was:\n\n" + asked[len(asked)-2] +
		"\n\nAnswer it now, properly, without asking what was meant.", nil
}
