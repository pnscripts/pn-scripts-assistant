package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

/*
 * Telling the brain to stop hearing this machine.
 *
 * The complaint it answers is one sentence long: music is playing and the
 * brain is confused. It is confused for a plain reason — the canceller
 * subtracts from the microphone whatever was played into its own sink, which
 * is the brain's voice and nothing else, so a browser playing music goes
 * straight to the speakers and the microphone hears every note of it mixed
 * with whoever is talking.
 *
 * The fix is to route the machine's output through the canceller, and it is a
 * tool rather than only a switch because the moment somebody wants it is the
 * moment they are already talking to a confused assistant over the top of
 * their own music.
 *
 * Mutating, and rightly: it changes where every program on this machine sends
 * its sound. That is not something to do because a sentence was misheard as a
 * request.
 */
type Quieten struct {
	// Reroute turns it on or off, and Rerouting says how it stands.
	Reroute   func(ctx context.Context, on bool) error
	Rerouting func() bool
}

func (Quieten) Name() string { return "stop_hearing_this_machine" }

func (Quieten) Description() string {
	return "Route everything this machine plays — music, videos, calls — through the echo " +
		"canceller, so the microphone stops hearing it mixed with your voice. Use this " +
		"when told that music is confusing it, that it keeps answering a video, or asked " +
		"to stop listening to the machine's own sound. Pass off to put it back."
}

func (Quieten) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"off":{"type":"boolean","description":"Put the sound back the way it was."}
		},
		"additionalProperties":false
	}`)
}

func (Quieten) Risk() Risk { return Mutating }

func (Quieten) Summarize(raw json.RawMessage) string {
	var a struct {
		Off bool `json:"off"`
	}

	json.Unmarshal(raw, &a)

	if a.Off {
		return "Send this machine's sound back to the speakers directly"
	}

	return "Send this machine's sound through the echo canceller, so the microphone stops hearing it"
}

func (t Quieten) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Off bool `json:"off"`
	}

	json.Unmarshal(raw, &a)

	if t.Reroute == nil {
		return "", fmt.Errorf("this machine's sound cannot be rerouted from here")
	}

	if err := t.Reroute(ctx, !a.Off); err != nil {
		return "", err
	}

	if a.Off {
		return "This machine's sound goes straight to the speakers again, so I hear it " +
			"through the microphone along with everything else in the room.", nil
	}

	return "Everything this machine plays now goes through the canceller, so I no longer " +
		"hear it. Anything playing from somewhere else — a phone, a radio — is still " +
		"sound in the room and I cannot subtract that.", nil
}
