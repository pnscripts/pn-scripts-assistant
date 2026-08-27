package speech

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"os/exec"
)

// Voice is something the brain can be read aloud by.
type Voice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Engine string `json:"engine"`
	Path   string `json:"-"`
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

	return available[0]
}

// Voices lists everything installed that can speak.
//
// Piper models first, because they are neural and sound it. espeak-ng appears
// last and only as a single entry rather than its hundreds of language
// variants, which would bury four good voices in a list nobody would read.
func Voices() []Voice {
	var out []Voice

	if p := FindPiper(); p != nil {
		for _, path := range piperVoiceFiles(p) {
			id := strings.TrimSuffix(filepath.Base(path), ".onnx")

			out = append(out, Voice{
				ID:     id,
				Name:   humaniseVoice(id),
				Engine: "piper",
				Path:   path,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	if e := fallbackEngine(); e != nil {
		out = append(out, Voice{
			ID:     "system",
			Name:   "System voice (" + e.Name + ")",
			Engine: e.Name,
		})
	}

	return out
}

// piperVoiceFiles finds every model beside a piper installation.
func piperVoiceFiles(p *Piper) []string {
	dirs := []string{
		filepath.Join(filepath.Dir(p.Binary), "voices"),
		filepath.Dir(p.Binary),
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
		if _, err := exec.LookPath(engines[i].Command); err != nil {
			continue
		}

		if engines[i].NeedsEngine && !anyVoiceInstalled() {
			continue
		}

		return &engines[i]
	}

	return nil
}
