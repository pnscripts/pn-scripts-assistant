package speech

import "testing"

/*
 * "Thanks for watching." arrived in Petar's conversation while a film was on
 * and nobody had spoken. Whisper was trained on subtitles, so where there are
 * no words it returns the likeliest thing to appear in that position.
 */
func TestSubtitleBoilerplateIsNotSomebodyTalking(t *testing.T) {
	for _, said := range []string{
		"Thanks for watching.",
		"thanks for watching",
		"Thank you for watching!",
		"  Thanks for watching!  ",
		"Please subscribe to my channel",
		"Subtitles by the Amara.org community",
		"Благодаря за гледането",
		"Абонирайте се",
	} {
		if !Ghost(said) {
			t.Errorf("%q was taken as something he said", said)
		}
	}
}

/*
 * And the other way, which matters more.
 *
 * An assistant that ignores being thanked is its own bug, and a sentence that
 * merely contains one of these phrases has other words in it — other words
 * mean somebody was talking. The ghost only ever appears alone.
 */
func TestRealSentencesAreNotMistakenForBoilerplate(t *testing.T) {
	for _, said := range []string{
		"thank you",
		"thanks",
		"Thank you, that was helpful.",
		"thanks for watching my calendar for me",
		"remind me to thank Ivan for watching the dog",
		"subscribe me to the newsletter about Laravel",
		"what are you watching",
		"",
		"   ",
	} {
		if Ghost(said) {
			t.Errorf("%q was thrown away as subtitle noise", said)
		}
	}
}
