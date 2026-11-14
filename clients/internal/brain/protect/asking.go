package protect

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

/*
 * Finding the protected thing in a tool call.
 *
 * Every tool names its argument differently — path, file, directory, folder —
 * and a list of which tool uses which name would be a list to keep up to date
 * and to get wrong. So the arguments are searched rather than interpreted:
 * every string in them is checked, whatever it is called.
 *
 * The bias is deliberate. A string that merely looks like a protected path
 * produces a question, and a question is answered in two seconds and then
 * remembered; the other kind of mistake is a credentials file read silently.
 */

// InArguments finds the first protected path a tool call names.
func InArguments(raw json.RawMessage) (path string, rule Rule, ask bool) {
	var anything any

	if err := json.Unmarshal(raw, &anything); err != nil {
		return "", Rule{}, false
	}

	return walk(anything)
}

func walk(value any) (string, Rule, bool) {
	switch v := value.(type) {
	case string:
		if r, ask := Ask(v); ask {
			return v, r, true
		}

	case []any:
		for _, item := range v {
			if path, r, ask := walk(item); ask {
				return path, r, true
			}
		}

	case map[string]any:
		/*
		 * In a settled order, so the same call asks the same question twice.
		 *
		 * Go randomises map iteration, so a call naming two protected files
		 * would name one of them in the prompt on Tuesday and the other on
		 * Wednesday — and a prompt that says something different each time
		 * about the same request is one nobody can learn to trust.
		 */
		for _, key := range sortedKeys(v) {
			if path, r, ask := walk(v[key]); ask {
				return path, r, true
			}
		}
	}

	return "", Rule{}, false
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))

	for k := range m {
		out = append(out, k)
	}

	// Small maps; a plain insertion sort keeps this file free of imports it
	// does not otherwise need.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}

/*
 * Explain is the question, written for the person who has to answer it.
 *
 * Three things, because with fewer than three there is no way to decide: what
 * the brain wants to do, which file, and why that file is one it stops at. A
 * prompt that says only "allow access?" teaches people to say yes.
 */
func Explain(what, path string, r Rule) string {
	where := path

	if home := strings.TrimSpace(r.What); home == "" {
		return fmt.Sprintf("%s: %s", what, where)
	}

	return fmt.Sprintf("%s: %s (%s). %s", what, where, strings.ToLower(r.What), r.Why)
}

/*
 * ExplainIn is Explain in a language, for the approval card: the rule's
 * name and reason in Bulgarian are the ones the Privacy page shows
 * ('prv.rule.<id>' in i18n.js).
 */
func ExplainIn(what, path string, r Rule, lang string) string {
	if lang != "bg" {
		return Explain(what, path, r)
	}

	bg, ok := rulesBG[r.ID]

	switch {
	case r.ID == "yours":
		bg = [2]string{"Нещо, което сте добавили",
			"Помолихте да ви се казва, преди нещо тук да бъде отворено: " + strings.TrimPrefix(r.Why, yoursWhy)}
	case !ok || strings.TrimSpace(r.What) == "":
		return fmt.Sprintf("%s: %s", what, path)
	}

	return fmt.Sprintf("%s: %s (%s). %s", what, path, lowerFirst(bg[0]), bg[1])
}

// lowerFirst lowers the first letter of a name, unless it is part of an
// abbreviation ("SSH ключове" stays as it is).
func lowerFirst(s string) string {
	runes := []rune(s)
	if len(runes) > 1 && unicode.IsUpper(runes[1]) {
		return s
	}

	if len(runes) > 0 {
		runes[0] = unicode.ToLower(runes[0])
	}

	return string(runes)
}

// rulesBG is each built-in rule's name and reason in Bulgarian, as the
// Privacy page says them.
var rulesBG = map[string][2]string{
	"ssh": {"SSH ключове",
		"Частни ключове за всеки компютър, в който влизате. Копие на ключа е копие на достъпа ви до него."},
	"env": {"Файлове с променливи на средата",
		"Файлът .env съдържа паролите и API ключовете на проекта, към който принадлежи, като обикновен текст."},
	"cloud": {"Облачни профили",
		"Данни за вход в Amazon, Azure, Google и Kubernetes: достатъчни, за да се харчат пари от ваше име."},
	"registries": {"Данни за вход в регистри и мениджъри на пакети",
		"Docker, npm и git пазят данните си за вход нешифровани. Изтриването на инструмента не прави папката " +
			"безопасна. Прави я празна, а празното е временно."},
	"keys": {"Сертификати и файлове с ключове",
		"Всичко, което завършва на .pem, .key, .p12 или .pfx, по определение е ключ."},
	"passwords": {"Мениджъри на пароли и ключодържатели",
		"Ключодържателят на GnuPG и хранилището на pass са оригиналите, а не тяхно копие."},
	"databases": {"Пароли за бази данни",
		"Файловете, в които клиентите на Postgres и MySQL пазят пароли, за да не ги въвеждате."},
	"browsers": {"Профили на браузъри",
		"Запазени пароли, бисквитки и сесии за всеки сайт, в който сте влезли."},
	"brain": {"Папката на самия мозък",
		"Файлът с настройките му пази паролата за пощата и всеки API ключ, който му е даден, като обикновен текст, " +
			"а паметта му е всичко, което някога му е казано. Мозъкът чете паметта си със собствените си " +
			"инструменти, а това е различно от отваряне на файла и поставяне на съдържанието му в разговор."},
	"system": {"Файлът с паролите на системата",
		"/etc/shadow съдържа хеша на паролата на всеки потребител. Нито една основателна заявка към асистента не се нуждае от него."},
}
