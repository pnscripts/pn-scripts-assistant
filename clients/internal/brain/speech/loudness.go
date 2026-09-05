package speech

import (
	"context"
	"fmt"
	"sync/atomic"
)

/*
 * How loud its own voice is, and when it is quieter than that.
 *
 * Every other program on this machine has a volume and this one did not: the
 * assistant played at exactly the same level as whatever else was going on,
 * and the only way to make it quieter was to turn the speakers down and lose
 * the music with it.
 *
 * The second half matters more than the first. Asked for a lower voice,
 * somebody almost never means "lower for everything" — they mean the things it
 * says on its own should not arrive at the same level as the answer to a
 * question they just asked. A greeting, a note that forty documents have been
 * read, a remark nobody was waiting for: those can be under the room. An
 * answer, and a reminder that has come due, should not be.
 */
const (
	// FullLevel is the loudest it will play, which is the system's own level.
	FullLevel = 1.0

	// QuietestUseful is below which it can be heard to be talking and not
	// understood, which is worse than silence.
	QuietestUseful = 0.2

	/*
	 * UnpromptedShare is how much of its voice an unasked-for remark gets.
	 *
	 * Not silence: a brain that learns something and says so at a whisper is
	 * still saying it, and the whole point of speaking unprompted is that
	 * somebody is doing something else. Under the room rather than over it.
	 */
	UnpromptedShare = 0.55
)

var loudness atomic.Uint64

func init() { SetLoudness(FullLevel) }

// SetLoudness sets the level for answers, bounded so that "quieter" can never
// mean inaudible and "louder" can never mean distorted.
func SetLoudness(level float64) {
	if level > FullLevel {
		level = FullLevel
	}

	if level < QuietestUseful {
		level = QuietestUseful
	}

	loudness.Store(uint64(level * 1000))
}

// Loudness is the level answers are spoken at.
func Loudness() float64 { return float64(loudness.Load()) / 1000 }

/*
 * unprompted marks the utterance in flight as one nobody asked for.
 *
 * Carried on the context rather than passed down, because speaking runs
 * through several layers that have no business knowing why they were called —
 * and the alternative was a parameter threaded through a dozen call sites,
 * eleven of which would pass the wrong thing eventually.
 */
type unpromptedKey struct{}

// Unasked marks a context as carrying something said on the brain's own
// initiative, which is spoken under the room rather than over it.
func Unasked(ctx context.Context) context.Context {
	return context.WithValue(ctx, unpromptedKey{}, true)
}

// LevelFor is how loud this utterance should be.
func LevelFor(ctx context.Context) float64 {
	if was, _ := ctx.Value(unpromptedKey{}).(bool); was {
		return Loudness() * UnpromptedShare
	}

	return Loudness()
}

// volumeArgs is how a player is told the level, where it can be told at all.
func volumeArgs(level float64) []string {
	if level >= FullLevel {
		return nil
	}

	return []string{"--volume", fmt.Sprintf("%.3f", level)}
}
