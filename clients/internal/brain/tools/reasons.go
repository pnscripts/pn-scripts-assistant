package tools

import "strings"

/*
 * Why a call is always asked, in Bulgarian.
 *
 * The reasons (Consent, AlwaysAsks, Destructive, goal mode) stay English
 * where they are made, because the loop and goal mode read them; the card
 * says them in the page's language through this table. A reason the table
 * does not know is said as it came, and TestEveryReasonIsSaidInBulgarian
 * fails before that can reach a card.
 */
var reasonsBG = map[string]string{
	"sending a message in your name":                                       "изпращане на съобщение от ваше име",
	"installing software":                                                  "инсталиране на софтуер",
	"downloading a model onto this machine":                                "изтегляне на модел на този компютър",
	"removing software from this machine":                                  "премахване на софтуер от този компютър",
	"it runs as the administrator":                                         "изпълнява се като администратор",
	"it deletes files for good":                                            "изтрива файлове завинаги",
	"it overwrites data in place":                                          "презаписва данни на място",
	"it stops programs":                                                    "спира програми",
	"it changes permissions on a whole tree":                               "променя правата на цяло дърво от папки",
	"it stops or disables a service":                                       "спира или изключва услуга",
	"it turns the machine off":                                             "изключва компютъра",
	"it throws away uncommitted work":                                      "изхвърля работа, която не е записана с commit",
	"it deletes untracked files for good":                                  "изтрива завинаги файлове, които Git не следи",
	"it rewrites or deletes history somewhere else":                        "пренаписва или изтрива историята на друго място",
	"it deletes a branch that may not be merged":                           "изтрива клон, който може да не е слят",
	"it throws away stashed work":                                          "изхвърля работа, оставена настрана със stash",
	"it throws away uncommitted changes":                                   "изхвърля промени, които не са записани с commit",
	"it rewrites history":                                                  "пренаписва историята",
	"it formats a disk":                                                    "форматира диск",
	HiddenCommand:                                                          "изпълнява скрита команда, която не може да се прочете, затова може да промени цялата система",
	"it stops a service or changes accounts, a change to the whole system": "спира услуга или променя потребителски профили, а това е промяна на цялата система",
	"it changes the whole system's registry":                               "променя регистъра на цялата система",
	"it changes the whole system's settings or permissions":                "променя настройките или правата на цялата система",
	"a permanent member of the organisation":                               "постоянен член на организацията",
	"switching an integration on":                                          "включване на интеграция",
	"adding an integration from a registry":                                "добавяне на интеграция от регистър",
	"a new project in your folders":                                        "нов проект в папките ви",
	"giving files and a question to Claude":                                "даване на файлове и въпрос на Claude",
	"stopping a program":                                                   "спиране на програма",
	"keeping a rule, which is followed from now on":                        "запазване на правило, което се спазва оттук нататък",
	"full access for a coding agent":                                       "пълен достъп за агент за програмиране",
	"held for your quiet hours":                                            "задържано за тихите ви часове",
	AskPaying:                                                              "плащане или платена услуга",
	AskSending:                                                             "изпращане или публикуване от ваше име",
	AskDeleting:                                                            "изтриване на файлове извън папката на целта",
	AskSystemWide:                                                          "промяна на цялата система",
	TypingInAnotherProgram:                                                 "писане и щракане в друга програма",
}

// TypingInAnotherProgram is why typing and clicking in another program's
// windows is always asked (see the agent loop).
const TypingInAnotherProgram = "typing and clicking in another program"

// ReasonIn is why a call is always asked, in a language. Several reasons
// joined with "; " are said one by one; "<reason> (<detail>)" says both.
func ReasonIn(why, lang string) string {
	if lang != "bg" || why == "" {
		return why
	}

	if strings.Contains(why, "; ") {
		parts := strings.Split(why, "; ")

		for i, p := range parts {
			parts[i] = ReasonIn(p, lang)
		}

		return strings.Join(parts, "; ")
	}

	if bg, ok := reasonsBG[why]; ok {
		return bg
	}

	// An integration that reaches off the machine names its server.
	if rest, ok := strings.CutPrefix(why, "sending data to "); ok {
		if server, ok := strings.CutSuffix(rest, ", off this machine"); ok {
			return "изпращане на данни към " + server + ", извън този компютър"
		}
	}

	if open := strings.Index(why, " ("); open > 0 && strings.HasSuffix(why, ")") {
		return ReasonIn(why[:open], lang) + " (" + ReasonIn(why[open+2:len(why)-1], lang) + ")"
	}

	return why
}
