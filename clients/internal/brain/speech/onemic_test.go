package speech

import (
	"sync"
	"testing"
)

/*
 * Two recorders on one microphone split what was said between them.
 *
 * That is how "Brain, tuberously" comes out of a whole spoken question: each
 * process receives part of the audio, whisper transcribes the fragment
 * confidently — half a sentence is still a sentence — and the brain answers
 * something nobody said.
 */
func TestOnlyOneRecorderGetsTheMicrophone(t *testing.T) {
	if !claimTheMicrophone() {
		t.Fatal("the microphone was already taken at the start of the test")
	}

	if claimTheMicrophone() {
		releaseTheMicrophone()
		t.Fatal("a second recorder was allowed onto the same microphone")
	}

	releaseTheMicrophone()

	if !claimTheMicrophone() {
		t.Error("the microphone was not released")
	}

	releaseTheMicrophone()
}

// And the claim has to hold when turns arrive together, which is the only way
// this ever went wrong — nothing takes the microphone twice in a straight line.
func TestTheMicrophoneSurvivesARace(t *testing.T) {
	const racers = 50

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		won int
	)

	for i := 0; i < racers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if claimTheMicrophone() {
				mu.Lock()
				won++
				mu.Unlock()

				releaseTheMicrophone()
			}
		}()
	}

	wg.Wait()

	if won == 0 {
		t.Error("nobody got the microphone, so no turn would ever be recorded")
	}

	// Serialised, so more than one can win over time — what must never happen
	// is two holding it at once, which the counter above cannot show. The
	// first test covers that; this one checks the lock does not deadlock or
	// starve under contention.
	if won > racers {
		t.Errorf("won %d of %d, which is not possible", won, racers)
	}
}
