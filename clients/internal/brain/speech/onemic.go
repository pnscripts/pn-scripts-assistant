package speech

import (
	"errors"
	"sync"
)

/*
 * There is one microphone, so there is one recorder.
 *
 * Nothing enforced that, and two turn recorders were found running at the same
 * time on this machine: one bound to the echo-cancelled source, one that had
 * failed to resolve a device and taken the raw default instead. Both pulling
 * from the same hardware, each receiving part of what was said, each handing
 * whisper a fragment — which is transcribed confidently, because half a
 * sentence is still a sentence, and then answered as though somebody had said
 * it.
 */

// ErrAlreadyRecording means a turn is already being recorded.
var ErrAlreadyRecording = errors.New("a turn is already being recorded")

var theMicrophone struct {
	mu    sync.Mutex
	taken bool
}

// claimTheMicrophone takes the microphone, or reports that somebody else has
// it. Never blocks: waiting for the previous turn would mean recording after
// the person stopped talking.
func claimTheMicrophone() bool {
	theMicrophone.mu.Lock()
	defer theMicrophone.mu.Unlock()

	if theMicrophone.taken {
		return false
	}

	theMicrophone.taken = true

	return true
}

func releaseTheMicrophone() {
	theMicrophone.mu.Lock()
	theMicrophone.taken = false
	theMicrophone.mu.Unlock()
}
