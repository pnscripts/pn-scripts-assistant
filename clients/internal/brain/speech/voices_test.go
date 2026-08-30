package speech

import "testing"

/*
 * "A woman's voice or a man's" is the question people actually have.
 *
 * "alba, amy, lessac, northern_english_male" is not an answer to it; it is a
 * list of names that has to be researched before it can be chosen from.
 */
func TestPickingAVoiceByKind(t *testing.T) {
	// The names carry no pattern, which is why this is a list and not a rule.
	women := []string{"en_GB-alba-medium", "en_US-amy-medium", "it_IT-paola-medium"}
	men := []string{"en_US-lessac-medium", "en_GB-northern_english_male-medium",
		"de_DE-thorsten-medium", "bg_BG-dimitar-medium"}

	for _, id := range women {
		if voiceSex[id] != "woman" {
			t.Errorf("%s is not marked as a woman's voice", id)
		}
	}

	for _, id := range men {
		if voiceSex[id] != "man" {
			t.Errorf("%s is not marked as a man's voice", id)
		}
	}

	// Nothing is guessed at: an unknown voice is offered without a description
	// rather than assigned one.
	if voiceSex["xx_XX-unknown-medium"] != "" {
		t.Error("a voice nobody has described was given a kind anyway")
	}
}
