package speech

import (
	"context"
	"testing"
)

/*
 * How loud its own voice is, and only its own.
 *
 * Every other program on this machine has a volume and this one did not, so
 * the only way to make the assistant quieter was to turn the speakers down and
 * lose the music with it.
 */
func TestTheLevelIsItsOwnStreamAndNothingElse(t *testing.T) {
	defer SetLoudness(FullLevel)

	SetLoudness(0.6)

	args := volumeArgs(Loudness())

	if len(args) != 2 || args[0] != "--volume" {
		t.Fatalf("the level is not passed to the player: %v", args)
	}

	// pw-play's --volume applies to the stream it creates, which is this
	// program's and no other. Nothing here touches a sink or a device.
	if args[1] != "0.600" {
		t.Fatalf("asked the player for %q", args[1])
	}
}

// At full it says nothing to the player, which is one less thing to be wrong
// on a machine whose player may not take a level at all.
func TestFullVolumePassesNothing(t *testing.T) {
	defer SetLoudness(FullLevel)

	SetLoudness(FullLevel)

	if args := volumeArgs(Loudness()); args != nil {
		t.Fatalf("passed %v when nothing was needed", args)
	}
}

/*
 * Bounded at both ends.
 *
 * Below a fifth it can be heard talking and not understood, which is worse
 * than silence — somebody knows it said something and has to ask again. Above
 * full is distortion rather than volume.
 */
func TestTheLevelCannotBeSetToUseless(t *testing.T) {
	defer SetLoudness(FullLevel)

	SetLoudness(0.0)

	if Loudness() != QuietestUseful {
		t.Fatalf("silence was allowed: %v", Loudness())
	}

	SetLoudness(4)

	if Loudness() != FullLevel {
		t.Fatalf("went past full: %v", Loudness())
	}
}

/*
 * Not for everything.
 *
 * Asked for a lower voice, somebody almost never means lower for everything:
 * they mean the things it says on its own should not arrive at the level of an
 * answer they just asked for.
 */
func TestSomethingSaidUnpromptedIsQuieterThanAnAnswer(t *testing.T) {
	defer SetLoudness(FullLevel)

	SetLoudness(FullLevel)

	answer := LevelFor(context.Background())
	remark := LevelFor(Unasked(context.Background()))

	if remark >= answer {
		t.Fatalf("an unasked-for remark was as loud as an answer: %v against %v",
			remark, answer)
	}

	// Under the room, not inaudible: a brain that learns something and says so
	// at a whisper is still saying it.
	if remark < answer*0.4 {
		t.Fatalf("an unasked-for remark was reduced to a whisper: %v", remark)
	}
}

// And lowering the voice lowers both, since one is a share of the other.
func TestTurningItDownTurnsBothDown(t *testing.T) {
	defer SetLoudness(FullLevel)

	SetLoudness(FullLevel)

	wasRemark := LevelFor(Unasked(context.Background()))

	SetLoudness(0.5)

	if LevelFor(Unasked(context.Background())) >= wasRemark {
		t.Fatal("turning the voice down left unprompted remarks where they were")
	}
}
