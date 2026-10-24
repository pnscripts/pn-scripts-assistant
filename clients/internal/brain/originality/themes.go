package originality

// Theme is a well-known melody a new one must not repeat.
//
// Only works in the public domain are bundled — so the reference set itself
// can ship — and only their best-known opening phrase, which is what a
// listener recognises. Public domain does not make them ours: a "new" game
// tune that is Ode to Joy is still not original, and some (Korobeiniki) are
// bound up with a famous product's identity.
type Theme struct {
	Name, By, Note string
	Notes          string // note names with octave, space separated
}

// Themes is the bundled reference set.
var Themes = []Theme{
	{Name: "Ode to Joy", By: "Beethoven, Symphony No. 9 (1824)",
		Notes: "E4 E4 F4 G4 G4 F4 E4 D4 C4 C4 D4 E4 E4 D4 D4 E4 E4 F4 G4 G4 F4 E4 D4 C4 C4 D4 E4 D4 C4 C4"},
	{Name: "Für Elise", By: "Beethoven (1810)",
		Notes: "E5 D#5 E5 D#5 E5 B4 D5 C5 A4 C4 E4 A4 B4 E4 G#4 B4 C5"},
	{Name: "Twinkle, Twinkle, Little Star (Ah! vous dirai-je, maman)", By: "French folk tune (18th c.)",
		Notes: "C4 C4 G4 G4 A4 A4 G4 F4 F4 E4 E4 D4 D4 C4"},
	{Name: "Frère Jacques", By: "French folk tune (18th c.)",
		Notes: "C4 D4 E4 C4 C4 D4 E4 C4 E4 F4 G4 E4 F4 G4"},
	{Name: "Greensleeves", By: "English folk tune (16th c.)",
		Notes: "A4 C5 D5 E5 F5 E5 D5 B4 G4 A4 B4 C5 A4 A4 G#4 A4 B4 G#4 E4"},
	{Name: "In the Hall of the Mountain King", By: "Grieg, Peer Gynt (1875)",
		Notes: "B3 C#4 D4 E4 F#4 D4 F#4 F4 C#4 F4 E4 C4 E4 B3 C#4 D4 E4 F#4 D4 F#4 B4 A4 F#4 D4 F#4 A4"},
	{Name: "Korobeiniki", By: "Russian folk song (1861)", Note: "known worldwide as the Tetris theme",
		Notes: "E5 B4 C5 D5 C5 B4 A4 A4 C5 E5 D5 C5 B4 C5 D5 E5 C5 A4 A4"},
	{Name: "Eine kleine Nachtmusik", By: "Mozart (1787)",
		Notes: "G4 D4 G4 D4 G4 D4 G4 B4 D5 C5 A4 C5 A4 C5 A4 F#4 A4 D4"},
	{Name: "Jingle Bells", By: "James Lord Pierpont (1857)",
		Notes: "E4 E4 E4 E4 E4 E4 E4 G4 C4 D4 E4 F4 F4 F4 F4 F4 E4 E4 E4 E4 D4 D4 E4 D4 G4"},
	{Name: "Mary Had a Little Lamb", By: "American nursery tune (1830s)",
		Notes: "E4 D4 C4 D4 E4 E4 E4 D4 D4 D4 E4 G4 G4 E4 D4 C4 D4 E4 E4 E4 E4 D4 D4 E4 D4 C4"},
	{Name: "Lullaby (Wiegenlied)", By: "Brahms (1868)",
		Notes: "E4 E4 G4 E4 E4 G4 E4 G4 C5 B4 A4 A4 G4 D4 E4 F4 D4 D4 E4 F4 D4 F4 B4 A4 G4 B4 C5"},
	{Name: "The Entertainer", By: "Scott Joplin (1902)",
		Notes: "D4 D#4 E4 C5 E4 C5 E4 C5 C5 D5 D#5 E5 C5 D5 E5 B4 D5 C5"},
	{Name: "Canon in D (ground bass)", By: "Pachelbel (c. 1700)",
		Notes: "D3 A2 B2 F#2 G2 D2 G2 A2 D3 A2 B2 F#2 G2 D2 G2 A2"},
	{Name: "Happy Birthday to You", By: "Hill sisters' tune (1893), public domain",
		Notes: "G4 G4 A4 G4 C5 B4 G4 G4 A4 G4 D5 C5 G4 G4 G5 E5 C5 B4 A4 F5 F5 E5 C5 D5 C5"},
	{Name: "Auld Lang Syne", By: "Scottish folk tune (18th c.)",
		Notes: "C4 F4 F4 F4 A4 G4 F4 G4 A4 F4 F4 A4 C5 D5 D5 C5 A4 A4 F4 G4 F4 G4 A4 F4 D4 D4 C4 F4"},
	{Name: "Amazing Grace", By: "American hymn tune (1835)",
		Notes: "D4 G4 B4 G4 B4 A4 G4 E4 D4 D4 G4 B4 G4 B4 A4 D5"},
	{Name: "The Blue Danube", By: "Johann Strauss II (1866)",
		Notes: "D4 D4 F#4 A4 A4 A5 A5 F#5 F#5 D4 D4 F#4 A4 A4 A5 A5 G5 G5"},
	{Name: "Dies irae", By: "Gregorian chant (13th c.)",
		Notes: "F4 E4 F4 D4 E4 C4 D4 D4 F4 F4 G4 F4 E4 D4 C4 E4 F4 E4 D4"},
	{Name: "Funeral March", By: "Chopin, Sonata No. 2 (1839)",
		Notes: "A#3 A#3 A#3 A#3 C#4 C4 C4 A#3 A#3 A3 A#3"},
	{Name: "London Bridge Is Falling Down", By: "English nursery tune",
		Notes: "G4 A4 G4 F4 E4 F4 G4 D4 E4 F4 E4 F4 G4"},
	{Name: "Old MacDonald Had a Farm", By: "American folk tune",
		Notes: "G4 G4 G4 D4 E4 E4 D4 B4 B4 A4 A4 G4"},
	{Name: "Row, Row, Row Your Boat", By: "American nursery tune (1852)",
		Notes: "C4 C4 C4 D4 E4 E4 D4 E4 F4 G4 C5 C5 C5 G4 G4 G4 E4 E4 E4 C4 C4 C4 G4 F4 E4 D4 C4"},
	{Name: "Yankee Doodle", By: "American folk tune (18th c.)",
		Notes: "C4 C4 D4 E4 C4 E4 D4 G3 C4 C4 D4 E4 C4 B3"},
	{Name: "Westminster Quarters", By: "chime (1793)", Note: "the best-known clock chime",
		Notes: "G#4 F#4 E4 B3 E4 G#4 F#4 B3 G#4 E4 F#4 B3 B3 F#4 G#4 E4"},
}
