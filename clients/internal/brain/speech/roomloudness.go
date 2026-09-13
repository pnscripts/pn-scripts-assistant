package speech

import "sync/atomic"

/*
 * How loud to speak, decided by how loud the room is.
 *
 * A fixed level is right in exactly one room. The same setting that carries
 * across a kitchen with a extractor fan running is a shout in a quiet study at
 * night, and somebody who turns it down for the evening has to turn it back up
 * in the morning — which is the sort of small tax that ends with the voice
 * switched off altogether.
 *
 * So the room is measured and the voice follows it. The measurement is free:
 * every recording is already levelled, because "heard nothing" has to be able
 * to tell a silent microphone from a quiet room. A recording that came back
 * with no words in it *is* the room, and that is the number used.
 *
 * It only ever goes quieter than the level its owner set, never louder, and
 * that asymmetry is deliberate rather than a limitation. The setting is a
 * ceiling somebody chose; a program that decided on its own to exceed it would
 * be overruling them, and the failure would arrive at two in the morning at
 * full volume. Downward it can only ever be wrong by being too quiet, which is
 * a thing you notice and not a thing that wakes the house.
 */

const (
	/*
	 * QuietRoom and LoudRoom are the ends of the scale, in the same RMS units
	 * a recording is measured in.
	 *
	 * QuietRoom is a little above the noise floor of a real microphone in a
	 * still room — below it there is nothing to be heard over. LoudRoom is a
	 * room with something going on in it: a fan, a television, a conversation
	 * elsewhere. Above that it is already at the level that was set, and
	 * anything further is somebody else's job.
	 */
	QuietRoom = 150
	LoudRoom  = 1600

	/*
	 * InAQuietRoom is how much of the set level is used when the room is as
	 * still as it gets.
	 *
	 * Not a whisper. The voice still has to be understood from across the
	 * room, and a level nobody can make out is worse than one slightly louder
	 * than it needed to be — the second is mildly annoying and the first means
	 * asking it to repeat itself, which costs a whole answer.
	 */
	InAQuietRoom = 0.6

	/*
	 * Settling is how much of the new measurement is taken each time.
	 *
	 * Low, so that one lorry going past does not raise the voice for the rest
	 * of the evening, and one held breath does not lower it mid-sentence.
	 * Roughly: it takes about ten quiet stretches to move most of the way to a
	 * new room.
	 */
	Settling = 0.2
)

// The room as last measured, in RMS. Zero means it has not been heard yet, in
// which case the level that was set is used unchanged — a brain that has never
// listened has no business guessing at the room.
var room atomic.Uint64

/*
 * HeardTheRoom takes a measurement of a recording that had nothing said in it.
 *
 * Only those: a recording with speech in it measures the speaker, not the
 * room, and following that would make the voice track how loudly its owner
 * happens to be talking — which is the wrong thing and would oscillate.
 */
func HeardTheRoom(level Level) {
	heard := float64(level.RMS)

	if heard < 0 {
		return
	}

	was := float64(room.Load())

	// The first measurement is taken whole. Easing in from zero would mean the
	// first thing it ever says is at the quiet-room level whatever the room is
	// actually like.
	now := heard

	if was > 0 {
		now = was + (heard-was)*Settling
	}

	room.Store(uint64(now))
}

// RoomLevel is the room as last measured, for anything that wants to show it.
// Zero means it has not been heard.
func RoomLevel() int { return int(room.Load()) }

// ForgetTheRoom drops the measurement, for a microphone that has changed or a
// test that wants to start from nothing.
func ForgetTheRoom() { room.Store(0) }

/*
 * RoomShare is how much of the set level this room calls for.
 *
 * One when the room is loud or has never been heard, down to InAQuietRoom when
 * it is as still as it gets, and straight between the two in the middle.
 */
func RoomShare() float64 {
	heard := float64(room.Load())

	if heard <= 0 {
		return 1
	}

	if heard >= LoudRoom {
		return 1
	}

	if heard <= QuietRoom {
		return InAQuietRoom
	}

	along := (heard - QuietRoom) / (LoudRoom - QuietRoom)

	return InAQuietRoom + along*(1-InAQuietRoom)
}
