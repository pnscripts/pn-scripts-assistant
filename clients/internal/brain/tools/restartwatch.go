package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/machinecare"
)

// RestartWatching is what "tell me when I can restart" needs from the brain.
type RestartWatching interface {
	WatchForRestart(conversation int64, d time.Duration) (machinecare.Watch, bool, error)
	StopRestartWatch() (machinecare.Watch, bool)
}

/*
 * WatchForRestart is "tell me when I can restart": the machine is looked at
 * every few minutes until a restart is safe, then its user is told, out
 * loud and in this conversation. It writes only the assistant's own records
 * and never restarts anything, so it is Safe.
 */
type WatchForRestart struct{ Care RestartWatching }

func (WatchForRestart) Name() string { return "watch_for_restart" }

func (WatchForRestart) Description() string {
	return "When a restart is not safe yet (software installing, a driver not built for the new kernel, " +
		"the assistant's own work running), watch the computer and tell the user as soon as it is: out " +
		"loud and in this conversation. Looks every 2 minutes, for up to the hours given (default 4, at " +
		"most 12); one watch at a time. stop=true ends a watch. It never restarts anything."
}

func (WatchForRestart) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"hours": {"type": "number", "description": "How long to watch, 0.25 to 12. Default 4."},
			"stop": {"type": "boolean", "description": "End the watch going now instead."}
		},
		"required": []
	}`)
}

func (WatchForRestart) Risk() Risk { return Safe }

func (WatchForRestart) Cues() []string {
	return []string{"restart", "reboot", "рестарт", "рестартирам", "рестартира"}
}

type restartWatchArgs struct {
	Hours float64 `json:"hours"`
	Stop  bool    `json:"stop"`
}

func (WatchForRestart) Summarize(raw json.RawMessage) string {
	var a restartWatchArgs

	_ = json.Unmarshal(raw, &a)

	if a.Stop {
		return "Stop watching for a safe restart"
	}

	return "Tell you when a restart is safe"
}

func (t WatchForRestart) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if t.Care == nil {
		return "", errors.New("the machine cannot be looked at from here")
	}

	var a restartWatchArgs

	if len(raw) > 0 {
		if err := argsOf(raw, &a); err != nil {
			return "", err
		}
	}

	if a.Stop {
		if _, ok := t.Care.StopRestartWatch(); !ok {
			return "No restart was being watched for.", nil
		}

		return "Stopped watching for a safe restart.", nil
	}

	if math.IsNaN(a.Hours) || a.Hours < 0 || (a.Hours > 0 && a.Hours < 0.25) || a.Hours > 12 {
		return "", fmt.Errorf("a restart is watched for 0.25 to 12 hours, not %g", a.Hours)
	}

	w, already, err := t.Care.WatchForRestart(ConversationOf(ctx), time.Duration(a.Hours*float64(time.Hour)))
	if err != nil {
		return "", err
	}

	return restartWatchSentence(w, already), nil
}

// restartWatchSentence says how a watch stands, for the model to pass on.
func restartWatchSentence(w machinecare.Watch, already bool) string {
	until := w.Until.Local().Format("15:04")

	switch w.State {
	case machinecare.Safe, machinecare.CannotTell:
		return w.Said("en")
	case machinecare.Watching:
		var why []string
		for _, i := range w.InTheWay {
			why = append(why, machinecare.Briefly(i, false))
		}

		lead := "Watching: I look every 2 minutes until " + until + " and will say when a restart is safe."
		if already {
			lead = "Already watching since " + w.Since.Local().Format("15:04") + ", until " + until +
				"; I will say when a restart is safe."
		}

		if len(why) == 0 {
			return lead
		}

		return lead + " In the way now: " + strings.Join(why, "; ") + "."
	}

	return w.Said("en")
}
