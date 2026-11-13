/*
 * Package terms is the agreement a person accepts before the assistant does
 * anything for them.
 *
 * The assistant does not keep saying that it is an AI: it behaves as an
 * assistant, and says so when asked. What a person has to know before they
 * rely on it (that it is an AI system whose answers can be wrong, what stays
 * on the computer, that it acts for them, and that what it sends is marked)
 * is said once, here, in terms they accept with one button the first time
 * the program opens. That is the moment article 50(1) of the EU AI Act
 * (Regulation (EU) 2024/1689) asks for: clearly, and at the latest at the
 * first interaction.
 *
 * The words live here, in both languages, and nowhere else in the program:
 * the agreement page and the card that shows them again read them from the
 * server, and a test holds the documentation (docs/guide/terms.md and
 * terms.bg.md) to the same sentences.
 *
 * Acceptance is kept beside the brain, in AcceptedFile, with the version it
 * was given to. Version goes up when the terms change, and then the next
 * start asks again: an acceptance of older terms is not an acceptance of
 * these.
 */
package terms

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is the terms' own number. Raise it whenever a word below changes in
// a way somebody who accepted the old ones would want to read again.
const Version = 1

// Dated is when this version was written, as YYYY-MM-DD.
const Dated = "2026-11-13"

// AcceptedFile is the record of acceptance, in the brain's own folder: one per
// brain, so a brain carried to another computer has been agreed to already,
// and a second person with a brain of their own is asked for theirs.
const AcceptedFile = "terms-accepted.json"

// Section is one heading and what it says, in one language.
type Section struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// Text is the whole of the terms in one language.
type Text struct {
	Title    string    `json:"title"`
	Lead     string    `json:"lead"`
	Version  string    `json:"version"`
	Sections []Section `json:"sections"`
}

/*
 * The terms, English first and Bulgarian second, side by side so a sentence
 * changed in one is seen to need changing in the other.
 *
 * Every claim here is what the program does today, and the README says the
 * same: "out of the box everything is allowed" (ADR 0007), what leaves the
 * machine in each privacy mode, the daily update question, the marks of the
 * aidisclosure package, the MIT licence.
 */
var sections = [][2]Section{
	{
		{
			Heading: "It is an AI system",
			Body: "PN Scripts Assistant is an artificial intelligence (AI) system. What it says, " +
				"writes and does comes from a language model, not from a person. Its answers can " +
				"be wrong, incomplete or out of date, so check anything that matters (health, " +
				"money, law, safety) before you rely on it or send it.",
		},
		{
			Heading: "Това е система с изкуствен интелект",
			Body: "PN Scripts Assistant е система с изкуствен интелект (ИИ). Каквото казва, пише " +
				"и прави, идва от езиков модел, а не от човек. Отговорите му може да са грешни, " +
				"непълни или остарели, затова проверявайте важното (здраве, пари, право, " +
				"безопасност), преди да разчитате на него или да го изпратите.",
		},
	},
	{
		{
			Heading: "What stays on this computer",
			Body: "What it learns about you (conversations, memory, settings) is kept in its folder " +
				"on this computer or on the drive you chose, encrypted. It leaves the computer only " +
				"for what you use: with a hosted model, your message, the memory recalled for it and " +
				"your profile go to the provider you chose, under that provider's terms; reading the " +
				"web fetches the pages; mail goes through your mail server. In private mode it uses " +
				"only local models and does not read the web. Once a day it asks GitHub whether a " +
				"new version is out and sends nothing about you; that can be switched off.",
		},
		{
			Heading: "Какво остава на този компютър",
			Body: "Каквото научава за вас (разговори, памет, настройки), се пази шифровано в " +
				"неговата папка на този компютър или на избрания от вас диск. Излиза от компютъра " +
				"само за това, което използвате: с модел в облака съобщението ви, извиканата за " +
				"него памет и профилът ви отиват при избрания от вас доставчик според неговите " +
				"условия; четенето в интернет изтегля страниците; писмата минават през вашия " +
				"пощенски сървър. В поверителен режим използва само локални модели и не чете " +
				"интернет. Веднъж на ден проверява в GitHub дали има нова версия и не изпраща " +
				"нищо за вас; това може да се изключи.",
		},
	},
	{
		{
			Heading: "It acts for you",
			Body: "Unless you change it in Settings, it acts without asking first, including when " +
				"it sends mail, installs programs and changes files. Every action is recorded and " +
				"can be looked at again. You are responsible for what it does on your behalf and " +
				"for what you send.",
		},
		{
			Heading: "Действа от ваше име",
			Body: "Ако не промените това в настройките, действа, без да пита предварително, " +
				"включително когато изпраща писма, инсталира програми и променя файлове. Всяко " +
				"действие се записва и може да се прегледа. Вие отговаряте за това, което прави " +
				"от ваше име, и за това, което изпращате.",
		},
	},
	{
		{
			Heading: "What it sends is marked",
			Body: "Mail it sends carries the header AI-Disclosure: mode=ai-originated, and the PDF " +
				"and Word documents it writes say \"AI-generated\" in their properties, so others " +
				"can tell they were written with AI. The words themselves are not changed.",
		},
		{
			Heading: "Изпратеното е отбелязано",
			Body: "Писмата, които изпраща, носят заглавката AI-Disclosure: mode=ai-originated, а " +
				"PDF и Word документите, които пише, имат „AI-generated“ в свойствата си, за да " +
				"се знае, че са написани с изкуствен интелект. Самият текст не се променя.",
		},
	},
	{
		{
			Heading: "No warranty",
			Body: "It is free software under the MIT licence and comes as is, without warranty of " +
				"any kind, as that licence says.",
		},
		{
			Heading: "Без гаранция",
			Body: "Това е свободен софтуер с лиценз MIT и се предоставя такъв, какъвто е, без " +
				"каквато и да е гаранция, както гласи лицензът.",
		},
	},
	{
		{
			Heading: "When the terms change",
			Body: "A new version of the program may come with new terms; then it asks you again " +
				"before you go on. You can read these terms again at any time on the \"What it " +
				"runs on\" page, and in the documentation (docs/guide/terms.md).",
		},
		{
			Heading: "Когато условията се променят",
			Body: "Нова версия на програмата може да дойде с нови условия; тогава асистентът ще ви попита " +
				"отново, преди да продължите. Можете да прочетете тези условия отново по всяко " +
				"време в изгледа „На какво работи“ и в документацията (docs/guide/terms.bg.md).",
		},
	},
}

// In is the terms in a language: Bulgarian for "bg" (any case, any region),
// English otherwise.
func In(language string) Text {
	i := 0
	if bulgarian(language) {
		i = 1
	}

	t := Text{
		Title: [2]string{"Terms of use", "Условия за ползване"}[i],
		Lead: [2]string{
			"Please read these before you start. You accept them once; if they change, you are asked again.",
			"Прочетете условията, преди да започнете. Приемате ги веднъж; ако се променят, ще бъдете попитани отново.",
		}[i],
		Version: versionLine(i),
	}

	for _, s := range sections {
		t.Sections = append(t.Sections, s[i])
	}

	return t
}

func versionLine(i int) string {
	day, _ := time.Parse("2006-01-02", Dated)

	if i == 1 {
		return fmt.Sprintf("Версия %d, %s", Version, day.Format("02.01.2006"))
	}

	return fmt.Sprintf("Version %d, %s", Version, day.Format("2 January 2006"))
}

func bulgarian(language string) bool {
	l := strings.ToLower(strings.TrimSpace(language))

	return l == "bg" || strings.HasPrefix(l, "bg-") || strings.HasPrefix(l, "bg_")
}

// Record is what was accepted, when, and in which language it was read.
type Record struct {
	Version    int       `json:"version"`
	AcceptedAt time.Time `json:"accepted_at"`
	Language   string    `json:"language"`
}

// Load reads the record of acceptance in root. No file is no record and no
// error: a brain that has never been agreed to.
func Load(root string) (Record, error) {
	raw, err := os.ReadFile(filepath.Join(root, AcceptedFile))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, nil
	}

	if err != nil {
		return Record{}, err
	}

	var r Record
	if err := json.Unmarshal(raw, &r); err != nil {
		return Record{}, fmt.Errorf("the record of the accepted terms cannot be read: %w", err)
	}

	return r, nil
}

/*
 * Accepted is whether the terms in force have been accepted for the brain in
 * root. Anything that cannot be read counts as not accepted: the safe reading
 * of a damaged record is to ask again, which costs one button.
 */
func Accepted(root string) bool {
	r, err := Load(root)

	return err == nil && r.Version >= Version
}

// ErrOtherVersion is an acceptance of terms that are not the ones in force:
// a page left open across an update, which showed the old words.
var ErrOtherVersion = errors.New("those are not the terms in force")

/*
 * Accept records that the terms of version were accepted at now, read in
 * language. Only the version in force can be accepted.
 *
 * Written to a temporary file and renamed, so a power cut leaves either the
 * old record or the new one and never half of one.
 */
func Accept(root string, version int, language string, now time.Time) (Record, error) {
	if version != Version {
		return Record{}, ErrOtherVersion
	}

	r := Record{Version: version, AcceptedAt: now.UTC().Truncate(time.Second), Language: "en"}
	if bulgarian(language) {
		r.Language = "bg"
	}

	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Record{}, err
	}

	tmp, err := os.CreateTemp(root, ".terms-*.json")
	if err != nil {
		return Record{}, fmt.Errorf("the acceptance could not be written: %w", err)
	}

	name := tmp.Name()

	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)

		return Record{}, fmt.Errorf("the acceptance could not be written: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)

		return Record{}, fmt.Errorf("the acceptance could not be written: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(name)

		return Record{}, fmt.Errorf("the acceptance could not be written: %w", err)
	}

	if err := os.Rename(name, filepath.Join(root, AcceptedFile)); err != nil {
		os.Remove(name)

		return Record{}, fmt.Errorf("the acceptance could not be written: %w", err)
	}

	return r, nil
}
