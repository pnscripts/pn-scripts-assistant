package speech

import (
	"context"
	"testing"
)

/*
 * A room that has never been heard does not change anything.
 *
 * The level somebody set is what they get until there is a reason to believe
 * otherwise. Guessing at a room from no measurement would be the program
 * inventing a number and then acting on it.
 */
func TestAnUnheardRoomChangesNothing(t *testing.T) {
	ForgetTheRoom()

	if got := RoomShare(); got != 1 {
		t.Errorf("an unheard room gave a share of %v", got)
	}
}

// A still room is spoken into more quietly than a busy one.
func TestAStillRoomIsSpokenIntoMoreQuietly(t *testing.T) {
	ForgetTheRoom()
	HeardTheRoom(Level{RMS: QuietRoom / 2})

	still := RoomShare()

	ForgetTheRoom()
	HeardTheRoom(Level{RMS: LoudRoom * 2})

	busy := RoomShare()

	if !(still < busy) {
		t.Errorf("a still room gave %v and a busy one %v", still, busy)
	}

	if still != InAQuietRoom {
		t.Errorf("the stillest room gave %v, want %v", still, InAQuietRoom)
	}

	if busy != 1 {
		t.Errorf("a busy room gave %v, want the level that was set", busy)
	}
}

/*
 * The room never makes it louder than the level its owner set.
 *
 * The asymmetry the whole thing rests on. A program that decided on its own to
 * go above the setting would be overruling somebody, and the failure arrives
 * at full volume in the middle of the night.
 */
func TestTheRoomNeverMakesItLouderThanTheSetting(t *testing.T) {
	SetLoudness(0.5)

	defer SetLoudness(FullLevel)

	for _, rms := range []int{0, 10, QuietRoom, LoudRoom, LoudRoom * 100} {
		ForgetTheRoom()
		HeardTheRoom(Level{RMS: rms})

		if got := LevelFor(context.Background()); got > 0.5 {
			t.Errorf("a room at %d was spoken into at %v, above the setting", rms, got)
		}
	}
}

// And never so quiet that it can be heard talking but not understood, which is
// worse than silence.
func TestItNeverDropsBelowBeingUnderstood(t *testing.T) {
	SetLoudness(QuietestUseful)

	defer SetLoudness(FullLevel)

	ForgetTheRoom()
	HeardTheRoom(Level{RMS: 1})

	if got := LevelFor(Unasked(context.Background())); got < QuietestUseful {
		t.Errorf("an unasked remark in a silent room was spoken at %v", got)
	}
}

/*
 * One loud moment does not raise the voice for the evening.
 *
 * A door closing, a lorry outside. The measurement settles towards the room
 * rather than jumping to whatever was heard last, and this is what stops the
 * voice tracking events instead of the room.
 */
func TestOneLoudMomentDoesNotMoveTheRoomMuch(t *testing.T) {
	ForgetTheRoom()

	for range 20 {
		HeardTheRoom(Level{RMS: QuietRoom})
	}

	settled := RoomLevel()

	HeardTheRoom(Level{RMS: LoudRoom * 4})

	if moved := RoomLevel() - settled; moved > (LoudRoom*4-settled)/2 {
		t.Errorf("one loud moment moved the room by %d", moved)
	}

	if RoomShare() == 1 {
		t.Error("one loud moment took the voice straight to full")
	}
}

// But a room that is genuinely louder is followed, given a little time.
func TestARoomThatIsGenuinelyLouderIsFollowed(t *testing.T) {
	ForgetTheRoom()

	for range 30 {
		HeardTheRoom(Level{RMS: LoudRoom})
	}

	if got := RoomShare(); got != 1 {
		t.Errorf("a genuinely loud room gave %v", got)
	}
}
