package speech

import (
	"os"
	"path/filepath"
	"pn-brain/internal/brain/exe"
	"sort"
	"strings"
	"sync"
)

// Voice is something the brain can be read aloud by.
type Voice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Engine string `json:"engine"`
	Path   string `json:"-"`

	// Sex is "woman", "man" or empty when it is not known.
	//
	// Here because "would you like a woman's voice or a man's" is the question
	// people actually have, and "alba, amy, lessac, northern_english_male" is
	// not an answer to it — it is a list of names that has to be researched
	// before it can be chosen from.
	Sex string `json:"sex,omitempty"`
}

/*
 * voiceSex is who each of the known voices sounds like.
 *
 * A list rather than a rule, because the names carry no pattern: alba and amy
 * are women, lessac and thorsten are men, and nothing about the words says so.
 * Anything not named here is offered without a description rather than guessed
 * at.
 */
var voiceSex = map[string]string{
	"en_GB-alba-medium":                 "woman",
	"en_US-amy-medium":                  "woman",
	"en_US-kathleen-low":                "woman",
	"en_GB-jenny_dioco-medium":          "woman",
	"en_GB-southern_english_female-low": "woman",
	"it_IT-paola-medium":                "woman",
	"fr_FR-siwis-medium":                "woman",

	"en_US-lessac-medium":                "man",
	"en_GB-northern_english_male-medium": "man",
	"en_US-ryan-medium":                  "man",
	"en_US-joe-medium":                   "man",
	"de_DE-thorsten-medium":              "man",
	"bg_BG-dimitar-medium":               "man",
	"ru_RU-dmitri-medium":                "man",
	"es_ES-davefx-medium":                "man",
}

/*
 * PickVoice returns the best installed voice of the kind asked for.
 *
 * Preferring one that matches the spoken language, because a British voice
 * reading Bulgarian is worse than either. Empty when there is none of that
 * kind, which the caller reports rather than silently substituting the other.
 */
func PickVoice(sex, language string) string {
	var fallback string

	for _, v := range Voices() {
		if v.Sex != sex {
			continue
		}

		if language != "" && strings.HasPrefix(strings.ToLower(v.ID), strings.ToLower(language)+"_") {
			return v.ID
		}

		if fallback == "" {
			fallback = v.ID
		}
	}

	return fallback
}

// chosenVoice is the one the owner picked, by ID. Empty means whatever the
// machine offers first.
var (
	chosenVoice string
	chosenMu    sync.RWMutex
)

// SetVoice chooses which voice speaks.
//
// Unknown ids are ignored rather than rejected: a voice can be deleted between
// the settings being written and the brain starting, and refusing to speak
// because a file moved would be a worse answer than using another one.
func SetVoice(id string) {
	chosenMu.Lock()
	chosenVoice = id
	chosenMu.Unlock()
}

// CurrentVoice reports which voice is in use.
// RobotVoice is the system speech engine, kept as the robot of last resort.
const RobotVoice = "system"

/*
 * RobotModel is the voice the robot is built out of, and RobotID is what the
 * robot is called in the list.
 *
 * The robot used to be espeak, which is genuinely a machine talking and is
 * also the reason nobody could make out what it said. Its owner asked for a
 * robot that is clear, and those are not opposites: the words and the timbre
 * come from different places. So the words come from the clearest neural voice
 * on the machine and the machine timbre is put on afterwards — see robot.go.
 *
 * espeak is still here, and still called a robot, for the machine that has no
 * neural voice installed. It is a worse robot, not a different kind of thing.
 */
const (
	RobotModel = "en_US-lessac-medium"
	RobotID    = "robot"
)

func CurrentVoice() Voice {
	available := Voices()

	if len(available) == 0 {
		return Voice{}
	}

	chosenMu.RLock()
	want := chosenVoice
	chosenMu.RUnlock()

	for _, v := range available {
		if v.ID == want {
			return v
		}
	}

	/*
	 * The robot, unless somebody has chosen otherwise.
	 *
	 * Falling through to the first voice in the list meant the default was
	 * whichever neural voice sorted first by name — alba, a woman, picked by
	 * the alphabet rather than by anybody. A machine voice is the honest
	 * default for a machine: it never pretends to be a person nobody chose,
	 * and the two human voices are one press away.
	 */
	for _, v := range available {
		if v.ID == RobotID || v.ID == RobotVoice {
			return v
		}
	}

	return available[0]
}

// Voices lists everything installed that can speak.
//
// Piper models first, because they are neural and sound it. espeak-ng appears
// last and only as a single entry rather than its hundreds of language
// variants, which would bury four good voices in a list nobody would read.
func Voices() []Voice {
	var out []Voice

	var robotBuiltFrom string

	if p := FindPiper(); p != nil {
		for _, path := range piperVoiceFiles(p) {
			id := strings.TrimSuffix(filepath.Base(path), ".onnx")

			/*
			 * The model the robot is made of is not offered under its own
			 * name.
			 *
			 * It would sit in the list as a third human voice while the robot
			 * built from it sat two rows below, and choosing between them
			 * would be choosing whether an effect is applied — which is not
			 * what the question "whose voice" is asking.
			 */
			if id == RobotModel {
				robotBuiltFrom = path

				continue
			}

			out = append(out, Voice{
				Sex:    voiceSex[id],
				ID:     id,
				Name:   humaniseVoice(id),
				Engine: "piper",
				Path:   path,
			})
		}
	}

	// The robot first in the list, since it is the one this answers in unless
	// somebody has said otherwise.
	if robotBuiltFrom != "" {
		out = append([]Voice{{
			ID:     RobotID,
			Sex:    "robot",
			Name:   "A robot",
			Engine: "piper",
			Path:   robotBuiltFrom,
		}}, out...)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	/*
	 * The system voice is a kind of voice, not the absence of one.
	 *
	 * espeak-ng is formant synthesis and sounds like a machine from 1985,
	 * which was treated here as a shortcoming to fall back to. It is also
	 * exactly what somebody means by "a robot voice", and for an assistant
	 * that is a choice rather than a consolation — so it is named for what it
	 * is and offered alongside the others.
	 */
	if e := fallbackEngine(); e != nil {
		name := "A robot (" + e.Name + ")"

		// Named for what it is when there is a better robot above it: the old
		// one, kept because it works where nothing else does.
		if robotBuiltFrom != "" {
			name = "An older robot (" + e.Name + ")"
		}

		out = append(out, Voice{
			ID:     RobotVoice,
			Sex:    "robot",
			Name:   name,
			Engine: e.Name,
		})
	}

	return out
}

/*
 * piperVoiceFiles finds every model beside a piper installation.
 *
 * Beside the real binary, not beside the name it was reached by. A piper
 * unpacked into ~/.local/share/piper and linked from ~/.local/bin is found
 * through the link, and the voices sit next to the target — so looking only
 * beside the link searched ~/.local/bin/voices, which does not exist, found
 * nothing, and left the system voice as the only one on offer. The neural
 * voice was installed, listed as installed by setup, and unreachable: the
 * assistant answered in the robotic fallback with nothing anywhere saying why.
 *
 * The standard locations are searched too, so a piper on PATH from a package
 * still finds voices downloaded into the usual place.
 */
func piperVoiceFiles(p *Piper) []string {
	dirs := []string{
		filepath.Join(filepath.Dir(p.Binary), "voices"),
		filepath.Dir(p.Binary),
	}

	if real, err := filepath.EvalSymlinks(p.Binary); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(real), "voices"), filepath.Dir(real))
	}

	if home, err := os.UserHomeDir(); err == nil {
		for _, dir := range piperSearch(home) {
			dirs = append(dirs, filepath.Join(dir, "voices"), dir)
		}
	}

	seen := map[string]bool{}

	var out []string

	for _, dir := range dirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.onnx"))

		for _, m := range matches {
			// The companion json holds the sample rate; piper cannot load a
			// model without it, so one without it is not a usable voice.
			if _, err := os.Stat(m + ".json"); err != nil {
				continue
			}

			if seen[filepath.Base(m)] {
				continue
			}

			seen[filepath.Base(m)] = true
			out = append(out, m)
		}
	}

	return out
}

// localeNames give a voice a place rather than a code.
var localeNames = map[string]string{
	"en_GB": "British", "en_US": "American", "en_AU": "Australian",
	"en_IE": "Irish", "en_IN": "Indian",
	"bg_BG": "Bulgarian", "ru_RU": "Russian", "uk_UA": "Ukrainian",
	"sr_RS": "Serbian", "mk_MK": "Macedonian",
	"de_DE": "German", "fr_FR": "French", "es_ES": "Spanish",
	"it_IT": "Italian", "pt_BR": "Brazilian", "pl_PL": "Polish",
	"nl_NL": "Dutch", "tr_TR": "Turkish", "ro_RO": "Romanian",
	"el_GR": "Greek", "cs_CZ": "Czech", "hu_HU": "Hungarian",
}

// humaniseVoice turns en_GB-northern_english_male-medium into something a
// person would pick from a list.
func humaniseVoice(id string) string {
	parts := strings.SplitN(id, "-", 3)

	if len(parts) < 2 {
		return id
	}

	locale, known := localeNames[parts[0]]
	if !known {
		// Better an unfamiliar code than a mangled one: "bg_BG" is at least
		// recognisable, where the previous fallback turned it into "bg BG".
		locale = parts[0]
	}

	name := strings.ReplaceAll(parts[1], "_", " ")
	name = strings.ToUpper(name[:1]) + name[1:]

	return name + " · " + locale
}

// fallbackEngine is whatever can speak without piper.
func fallbackEngine() *Engine {
	for i := range engines {
		if !exe.Has(engines[i].Command) {
			continue
		}

		if engines[i].NeedsEngine && !anyVoiceInstalled() {
			continue
		}

		return &engines[i]
	}

	return nil
}

/*
 * VoiceKind says which of the three a voice id is: robot, man or woman.
 *
 * So that "what are you set to" can be answered in the words the choice was
 * made in. The stored setting is a file path or an engine's own name, which is
 * the right thing to store and the wrong thing to say out loud.
 */
func VoiceKind(id string) string {
	if id == "" {
		return ""
	}

	if id == RobotID {
		return "robot"
	}

	for _, v := range Voices() {
		if v.ID == id || v.Path == id {
			if v.Sex != "" {
				return v.Sex
			}

			return v.Name
		}
	}

	return ""
}
