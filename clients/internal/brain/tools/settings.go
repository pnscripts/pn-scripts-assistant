package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

/*
 * Changing how it behaves, by saying so.
 *
 * Most of what somebody wants to adjust about an assistant they adjust while
 * talking to it — the voice is wrong, it keeps answering the television, it
 * should stop reading everything aloud. Those all lived in panels, which means
 * the answer to "how do I change this" was "stop talking to me and go and find
 * a page", from the program whose whole argument is that everything works from
 * inside it.
 *
 * Not everything belongs here. Privacy is set from setup and the privacy panel
 * and nowhere else, because a conversation that can talk the brain into
 * sharing more is a conversation somebody else can have with it. Asked, this
 * says so and says where to go, which is a better answer than doing it.
 */
type Settings struct {
	// Read hands back how things stand, so it can say what changed from what.
	Read func() Setting

	// Set applies one change, and returns what to say about it.
	Set func(ctx context.Context, what string, on bool) (string, error)

	// Voice changes which voice speaks: robot, man or woman.
	Voice func(ctx context.Context, which string) (string, error)

	// Loud changes how loud its own voice is, and only its own.
	Loud func(ctx context.Context, step string) (string, error)
}

// Setting is how the switches stand.
type Setting struct {
	SpeakAloud bool
	NameEvery  bool
	OnlyOwner  bool
	KeepQuiet  bool
	Cancelling bool

	// Voice is which of robot, man or woman is speaking.
	Voice string

	// Loudness is how loud its own voice is, from 0.2 to 1.
	Loudness float64
}

/*
 * switches are the things this can change, and what somebody calls them.
 *
 * Written out rather than matched loosely, because every entry is a change to
 * how the program behaves and a near-miss is a setting somebody did not ask
 * for. The key is what the tool takes; the words are what a person says.
 */
var switches = map[string][]string{
	"speak_aloud": {"speak", "talk", "read aloud", "out loud", "say your answers", "voice"},
	"name_every":  {"name every time", "always say your name", "wake word every"},
	"only_me":     {"only me", "only my voice", "ignore others", "just me"},
	"keep_quiet":  {"turn the rest down", "lower the music", "duck", "quieter while"},
	"cancel_room": {"stop hearing this machine", "cancel the room", "hear the music"},
}

func (Settings) Name() string { return "change_a_setting" }

func (Settings) Description() string {
	return "Change how the assistant behaves: whether it speaks its answers aloud, " +
		"whether it needs its name every time, whether it answers only its owner's " +
		"voice, whether it turns other sound down while it talks, whether it stops " +
		"hearing what this machine plays, and which voice it uses — robot, man or " +
		"woman. Use this whenever asked to change, turn on, turn off, or stop any of " +
		"those. It reports how they stand when asked without a change."
}

func (Settings) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"setting":{
				"type":"string",
				"description":"Which behaviour to change.",
				"enum":["speak_aloud","name_every","only_me","keep_quiet","cancel_room"]
			},
			"on":{"type":"boolean","description":"true to switch it on, false to switch it off."},
			"voice":{
				"type":"string",
				"description":"Change which voice speaks.",
				"enum":["robot","man","woman"]
			},
			"loudness":{
				"type":"string",
				"description":"Change how loud its own voice is, and nothing else on the machine. Use louder, quieter, full or lowest.",
				"enum":["louder","quieter","full","lowest"]
			}
		},
		"additionalProperties":false
	}`)
}

/*
 * Mutating: every one of these changes how the program behaves from then on.
 *
 * None of them is destructive and all are one sentence to undo, but a misheard
 * word must not be able to switch off somebody's microphone privacy without
 * their seeing it happen.
 */
func (Settings) Risk() Risk { return Mutating }

func (Settings) Summarize(raw json.RawMessage) string {
	var a settingArgs

	json.Unmarshal(raw, &a)

	if a.Voice != "" {
		return "Speak with the " + a.Voice + " voice"
	}

	if a.Loudness != "" {
		return "Speak " + a.Loudness + " — its own voice only"
	}

	if a.Setting == "" {
		return "Check how the settings stand"
	}

	if a.On == nil || !*a.On {
		return "Switch off: " + plainName(a.Setting)
	}

	return "Switch on: " + plainName(a.Setting)
}

type settingArgs struct {
	Setting  string `json:"setting"`
	On       *bool  `json:"on"`
	Voice    string `json:"voice"`
	Loudness string `json:"loudness"`
}

func (t Settings) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a settingArgs

	json.Unmarshal(raw, &a)

	if which := strings.TrimSpace(strings.ToLower(a.Voice)); which != "" {
		if t.Voice == nil {
			return "", fmt.Errorf("the voice cannot be changed from here")
		}

		return t.Voice(ctx, which)
	}

	if step := strings.TrimSpace(strings.ToLower(a.Loudness)); step != "" {
		if t.Loud == nil {
			return "", fmt.Errorf("the level cannot be changed from here")
		}

		return t.Loud(ctx, step)
	}

	if strings.TrimSpace(a.Setting) == "" {
		return t.howThingsStand(), nil
	}

	if _, known := switches[a.Setting]; !known {
		return fmt.Sprintf("I have no setting called %q. What I can change: %s. "+
			"Privacy is set in the privacy panel and nowhere else.",
			a.Setting, strings.Join(named(), ", ")), nil
	}

	if t.Set == nil {
		return "", fmt.Errorf("settings cannot be changed from here")
	}

	on := a.On != nil && *a.On

	return t.Set(ctx, a.Setting, on)
}

// howThingsStand is the answer to "what are you set to", which is asked far
// more often than any single change.
func (t Settings) howThingsStand() string {
	if t.Read == nil {
		return "I cannot see how the settings stand from here."
	}

	s := t.Read()

	lines := []string{
		yesNo("I speak my answers aloud", s.SpeakAloud),
		yesNo("I need my name every time", s.NameEvery),
		yesNo("I answer only your voice", s.OnlyOwner),
		yesNo("I turn other sound down while I talk", s.KeepQuiet),
		yesNo("I stop hearing what this machine plays", s.Cancelling),
	}

	said := strings.Join(lines, "\n")

	if s.Voice != "" {
		said += fmt.Sprintf("\nI am speaking with the %s voice.", s.Voice)
	}

	if s.Loudness > 0 {
		said += fmt.Sprintf(" My own voice is at %d%% — that is mine alone and "+
			"nothing else on this machine.", int(s.Loudness*100))
	}

	return said + "\n\nAny of those can be changed by saying so. Privacy is the one " +
		"thing I will not change from a conversation — that is set in the privacy panel."
}

func yesNo(what string, on bool) string {
	if on {
		return "· " + what + ": yes"
	}

	return "· " + what + ": no"
}

// plainName is what a switch is called in a sentence somebody reads before
// agreeing to it.
func plainName(key string) string {
	switch key {
	case "speak_aloud":
		return "speaking answers aloud"
	case "name_every":
		return "needing my name every time"
	case "only_me":
		return "answering only your voice"
	case "keep_quiet":
		return "turning other sound down while I talk"
	case "cancel_room":
		return "hearing what this machine plays"
	}

	return key
}

func named() []string {
	var out []string

	for key := range switches {
		out = append(out, plainName(key))
	}

	sort.Strings(out)

	return out
}
