package learning

import "strings"

/*
 * A subtitle is dialogue from a film, and it is never a memory.
 *
 * Petar asked whether the brain should learn from the subtitles on his film
 * drive. Two of the three things that question could mean are worth doing;
 * this is the one that is not, and it is worth writing down why rather than
 * only refusing.
 *
 * A subtitle says "Искаш да кажеш, че не ме позна?" — a line from The
 * Originals. Stored as a memory it becomes something the brain believes about
 * the person whose disk it is on, and it is not about them at all: it is a
 * sentence an actor said in a story. 866 films at a few lines each would put
 * thousands of them in front of everything the brain actually knows.
 *
 * And there is a worse failure than clutter, which this program has already
 * met both halves of. Whisper, given silence or film audio, returns the
 * likeliest thing to appear where there are no words — that is where "Thanks
 * for watching." came from. A brain that has also memorised film dialogue can
 * then find a matching memory for what it mishears out of the television, and
 * answer as though its owner had said it. The two mistakes are harmless
 * separately and feed each other together.
 *
 * Judged by shape rather than by extension, because subtitles arrive as .srt,
 * .sub, .vtt, .ass and as plain .txt, and only the last of those is currently
 * read at all. The shape is unmistakable and belongs to nothing else.
 */

// LooksLikeSubtitles reports whether text is a subtitle file's contents.
//
// The timecode line is the whole test: "00:00:03,956 --> 00:00:07,010" in SRT
// and WebVTT, "Dialogue: 0,0:00:03.95,..." in SSA. Nothing a person writes
// has those, and every subtitle format has one on every few lines.
func LooksLikeSubtitles(text string) bool {
	const enoughToBeSure = 3

	var timecodes int

	for i, line := range strings.Split(text, "\n") {
		// A subtitle declares itself in its first lines. Reading the whole of
		// a two-hour film to find that out would be the cost this avoids.
		if i > 200 {
			break
		}

		if strings.Contains(line, "-->") && strings.Contains(line, ":") {
			timecodes++
		}

		if strings.HasPrefix(strings.TrimSpace(line), "Dialogue: ") {
			timecodes++
		}

		if timecodes >= enoughToBeSure {
			return true
		}
	}

	return false
}
