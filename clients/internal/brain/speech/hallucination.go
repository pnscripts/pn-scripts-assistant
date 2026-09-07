package speech

import "strings"

/*
 * What whisper says when it has heard nothing.
 *
 * Every whisper model was trained on subtitles, and a large part of that
 * corpus ends the same way: somebody thanking you for watching, asking you to
 * subscribe, or crediting the subtitler. Given silence, music, or the hiss of
 * a room, the model does not return nothing — it returns the most likely thing
 * to appear where there are no words, which is one of those.
 *
 * It arrived in Petar's conversation as "Thanks for watching." while a film
 * was on and nobody had spoken. That is worse than a mis-transcription: it is
 * a sentence he never said, attributed to him, answered by the brain, and
 * eligible to be learned as something about him.
 *
 * The list is deliberately made of things nobody says to an assistant in their
 * own house. "Thank you" on its own is not here and must not be — people thank
 * it, and an assistant that ignores being thanked is its own bug.
 */
var subtitleGhosts = []string{
	"thanks for watching",
	"thank you for watching",
	"thanks for watching!",
	"thank you so much for watching",
	"thanks for watching and see you next time",
	"see you in the next video",
	"see you next video",
	"don't forget to subscribe",
	"please subscribe to my channel",
	"like and subscribe",
	"subscribe to my channel",
	"subtitles by the amara.org community",
	"subtitles by",
	"subtitling by",
	"transcription by",
	"amara.org",
	"www.mooji.org",
	"copyright",

	// Bulgarian subtitle boilerplate, which is what a film in this house is
	// most likely to be carrying.
	"благодаря за гледането",
	"благодаря ви за гледането",
	"абонирайте се",
	"субтитри от",
	"превод и субтитри",
}

/*
 * Ghost reports whether the whole transcript is subtitle boilerplate.
 *
 * The whole of it, never a part. A sentence that merely contains one of these
 * has other words in it, and other words mean somebody was talking — the ghost
 * appears alone, because it is what the model produces instead of nothing.
 */
func Ghost(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.Trim(t, " .!?,-–—\"'“”„…")
	t = strings.Join(strings.Fields(t), " ")

	if t == "" {
		return false
	}

	for _, ghost := range subtitleGhosts {
		if t == strings.Trim(ghost, " .!?") {
			return true
		}
	}

	return false
}
