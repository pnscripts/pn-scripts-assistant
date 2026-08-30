package speech

import "testing"

/*
 * Somebody who speaks the moment the microphone opens.
 *
 * This is the ordinary case for a brain that waits to be called by name: there
 * is no reason for its owner to pause politely before saying it, and every
 * reason to say it the instant they think of it.
 *
 * The room used to be measured from the first half-second only, so their voice
 * became the measurement. The bar was then set three and a half times above
 * their own speech and nothing they said afterwards could clear it. These are
 * the real numbers from the room this was found in: floor 1917, threshold
 * 6709, peak 3741 — a turn that heard somebody perfectly clearly and reported
 * that nothing had crossed the threshold.
 */
func TestSpeakingBeforeTheRoomHasBeenMeasured(t *testing.T) {
	const (
		room   = 420  // this room with nobody talking
		speech = 3741 // the peak actually measured when somebody did
	)

	// Four seconds in which somebody is talking almost throughout: loud
	// frames, with the short gaps that fall between words and sentences.
	var frames []int

	for i := 0; i < FloorWindow; i++ {
		if i%7 == 0 {
			frames = append(frames, room)

			continue
		}

		frames = append(frames, speech)
	}

	floor := quietest(frames, FloorQuiet)

	if floor > room*2 {
		t.Errorf("the room measured %d, which is their voice and not the room", floor)
	}

	if got := speechThreshold(floor); got > speech {
		t.Errorf("speech peaks at %d and the bar was set at %d, so they cannot be heard",
			speech, got)
	}

	// And somebody who barely draws breath: three gaps in four seconds, which
	// is the least the room is ever audible for.
	dense := make([]int, FloorWindow)

	for i := range dense {
		dense[i] = speech
	}

	dense[4], dense[19], dense[33] = room, room, room

	if got := speechThreshold(quietest(dense, FloorQuiet)); got > speech {
		t.Errorf("talking without pausing set the bar at %d, above the %d it was set from",
			got, speech)
	}
}

// And a genuinely quiet input still does not hear itself think.
func TestASilentInputDoesNotTriggerOnItsOwnHiss(t *testing.T) {
	var frames []int

	for i := 0; i < FloorWindow; i++ {
		frames = append(frames, 3+i%5)
	}

	if got := speechThreshold(quietest(frames, FloorQuiet)); got < MinSpeechFloor {
		t.Errorf("the bar fell to %d on a silent input, which its own hiss would clear", got)
	}
}

/*
 * A turn that cannot see the room still knows where it is.
 *
 * Somebody who talks for the whole four-second window leaves no quiet frame in
 * it, and the estimate would be their own voice — the same fault as measuring
 * only the first half second, just rarer. The room does not change between one
 * turn and the next, so the last good measurement carries over.
 */
func TestTheRoomIsRememberedBetweenTurns(t *testing.T) {
	const (
		room   = 70
		speech = 4000
	)

	remembered.mu.Lock()
	remembered.floor = 0
	remembered.mu.Unlock()

	// A turn that could see the room.
	rememberRoom(room)

	// The next one cannot: every frame of it is somebody talking.
	floor := roomFloor(speech)

	if bar := speechThreshold(floor); bar > speech {
		t.Errorf("a window of solid speech set the room to %d and the bar to %d, "+
			"which is above the speech it was measured from", floor, bar)
	}

	// A room that has genuinely got louder is followed, but not in one turn.
	rememberRoom(speech)

	if got := roomFloor(speech); got > room*2 {
		t.Errorf("one loud turn moved the room straight to %d", got)
	}

	for i := 0; i < 40; i++ {
		rememberRoom(speech)
	}

	if got := roomFloor(speech); got <= room {
		t.Errorf("a room that stayed loud was never followed: still %d", got)
	}
}

/*
 * Talking over a film.
 *
 * The room this is for has a television in it, so the floor is not a fan but
 * whatever is playing. Somebody talking over it is plainly louder than it and
 * nothing like several times louder: measured here at 1400 for the room and
 * 2500 to 4600 for a person, which the original margin of three and a half put
 * completely out of reach. Every turn reported that nothing had crossed the
 * threshold while somebody was talking into the microphone.
 */
func TestBeingHeardOverSomethingPlaying(t *testing.T) {
	const film = 1400

	// Every peak measured in that room while somebody was speaking.
	for _, spoke := range []int{2549, 2662, 2669, 2875, 2886, 2934, 3009, 3555, 3571, 3758} {
		if bar := speechThreshold(film); spoke < bar {
			t.Errorf("speech at %d over a room of %d needed %d, so it was not heard",
				spoke, film, bar)
		}
	}

	// A steady room tone is still not speech. A fan does not vary, so it never
	// reaches even this margin above itself.
	if bar := speechThreshold(film); film >= bar {
		t.Errorf("the room itself (%d) clears its own bar (%d)", film, bar)
	}
}

/*
 * An ordinary voice in a quiet room.
 *
 * The other half of the film problem, and it took the film stopping to see it.
 * With the room quiet the measured floor is around 40, so the margin gives a
 * bar of 68 — and the absolute minimum, written against a loud room, overrode
 * that with 500. Somebody talking normally a little away from the microphone
 * peaks at 300 to 630, so every word went under the bar and the brain reported
 * that nothing had crossed it.
 */
func TestBeingHeardInAQuietRoom(t *testing.T) {
	const quiet = 44

	// Every peak measured while somebody was speaking, with the room quiet.
	for _, spoke := range []int{283, 310, 323, 342, 352, 395, 398, 518, 630} {
		if bar := speechThreshold(quiet); spoke < bar {
			t.Errorf("speech at %d in a room of %d needed %d, so it was not heard",
				spoke, quiet, bar)
		}
	}

	// The room itself still does not clear its own bar, by a wide margin.
	if bar := speechThreshold(quiet); quiet*3 >= bar {
		t.Errorf("a room of %d is too close to the bar of %d; rustling would trigger it",
			quiet, bar)
	}

	// And a silent input does not hear its own hiss.
	if bar := speechThreshold(4); bar < 100 {
		t.Errorf("a silent input set the bar at %d, which its own hiss would clear", bar)
	}
}
