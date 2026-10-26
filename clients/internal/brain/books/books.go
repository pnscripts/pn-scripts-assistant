// Package books is an invoice register and a VAT helper: invoices checked
// as they are recorded, a period's VAT worked out from them, and the
// registration threshold watched — under a country's rules, each with the
// law it comes from and the date it was last checked.
//
// It gives information, not advice. A rule here is what the law said when
// it was checked, and the law changes; every figure it works out is shown
// beside the rule and the source it rests on, so its owner, or their
// accountant, can check it. Nothing here files anything with anybody.
package books

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/sheet"
)

// Source is where a rule comes from, and when it was last checked there.
type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	InForce string `json:"in_force,omitempty"`
	Checked string `json:"checked"`
}

// Rate is one VAT rate and what it is for.
type Rate struct {
	Value  string    `json:"value"` // percent, as written: "20", "9", "0"
	EN     string    `json:"en"`
	BG     string    `json:"bg"`
	Covers [2]string `json:"covers"` // EN, BG
	Rule   string    `json:"rule"`
}

// Pack is a country's rules.
type Pack struct {
	Country  string `json:"country"`
	EN       string `json:"en"`
	BG       string `json:"bg"`
	Currency string `json:"currency"`
	Rates    []Rate `json:"rates"`

	// Threshold is the yearly turnover above which a business must
	// register for VAT, in the currency; empty when the pack does not say.
	Threshold     string    `json:"threshold,omitempty"`
	ThresholdRule [2]string `json:"threshold_rule"`

	// The tax period is a calendar month; Due is the day of the next month
	// the return and the payment are due by.
	Due     int       `json:"due"`
	DueRule [2]string `json:"due_rule"`

	// InvoiceDays is how many days after the supply an invoice must be
	// issued by.
	InvoiceDays int       `json:"invoice_days"`
	InvoiceRule [2]string `json:"invoice_rule"`

	// Number is what an invoice number must look like, and VATID a VAT
	// number; nil checks nothing.
	Number     *regexp.Regexp `json:"-"`
	NumberRule [2]string      `json:"number_rule"`
	VATID      *regexp.Regexp `json:"-"`

	Fields  [2][]string `json:"fields"` // what an invoice must contain, EN and BG
	Sources []Source    `json:"sources"`
	Checked string      `json:"checked"`
}

// zddsURL is the Bulgarian VAT Act as published, with every amendment.
const zddsURL = "https://lex.bg/laws/ldoc/2135533201"

/*
 * Bulgaria, from the VAT Act (ЗДДС) as amended by ДВ бр. 115 of 2025, in
 * force from 1 January 2026, read on lex.bg on 26 October 2026. The rates
 * are чл. 66 and 66а; the threshold чл. 96(1); the monthly period чл. 87(2);
 * the return and the payment by the 14th, чл. 125(5) and чл. 89(1); the
 * invoice within five days, чл. 113(4); what an invoice must contain and
 * its ten-digit number, чл. 114(1).
 */
var bulgaria = Pack{
	Country: "bg", EN: "Bulgaria", BG: "България", Currency: "EUR",
	Rates: []Rate{
		{Value: "20", EN: "Standard rate", BG: "Стандартна ставка",
			Covers: [2]string{"Everything not named as taxed at a reduced or zero rate.", "Всичко, което не е изрично посочено като облагаемо с намалена или нулева ставка."},
			Rule:   "ЗДДС чл. 66, ал. 1"},
		{Value: "9", EN: "Reduced rate", BG: "Намалена ставка",
			Covers: [2]string{
				"Accommodation in hotels and similar places (including holiday lets and camping pitches); books and periodicals, printed or electronic; food for babies and small children, nappies and baby hygiene items (Annex 4).",
				"Настаняване в хотели и подобни заведения (включително ваканционно настаняване и места за къмпинг); книги и периодични издания на хартия или електронно; храни за бебета и малки деца, бебешки пелени и хигиенни артикули (приложение № 4)."},
			Rule: "ЗДДС чл. 66а"},
		{Value: "0", EN: "Zero rate", BG: "Нулева ставка",
			Covers: [2]string{"Exports, supplies to other EU countries and other cases the Act names; the invoice must say on what basis.",
				"Износ, доставки към други държави от ЕС и другите случаи, посочени в закона; фактурата трябва да посочва основанието."},
			Rule: "ЗДДС чл. 114, ал. 1, т. 12"},
	},
	Threshold:     "51130",
	ThresholdRule: [2]string{"Registration is compulsory when yearly turnover in the country exceeds €51,130 (ЗДДС чл. 96, ал. 1).", "Регистрацията е задължителна, когато годишният оборот в страната надвиши 51 130 евро (ЗДДС чл. 96, ал. 1)."},
	Due:           14,
	DueRule:       [2]string{"The tax period is the calendar month; the return is filed and the VAT paid by the 14th of the next month (ЗДДС чл. 87, чл. 125, ал. 5, чл. 89, ал. 1).", "Данъчният период е календарният месец; справка-декларацията се подава и данъкът се внася до 14-о число на следващия месец (ЗДДС чл. 87, чл. 125, ал. 5, чл. 89, ал. 1)."},
	InvoiceDays:   5,
	InvoiceRule:   [2]string{"An invoice is issued no later than 5 days after the supply or the advance payment (ЗДДС чл. 113, ал. 4).", "Фактурата се издава не по-късно от 5 дни от данъчното събитие или от получаването на аванса (ЗДДС чл. 113, ал. 4)."},
	Number:        regexp.MustCompile(`^\d{10}$`),
	NumberRule:    [2]string{"An invoice number has ten digits and nothing else, in one or more series (ЗДДС чл. 114, ал. 1, т. 2).", "Номерът на фактурата е десетразряден и съдържа само арабски цифри, в една или повече серии (ЗДДС чл. 114, ал. 1, т. 2)."},
	VATID:         regexp.MustCompile(`^BG\d{9,10}$`),
	Fields: [2][]string{
		{"the word Invoice", "a ten-digit number", "the date of issue", "the supplier's name and address", "the supplier's VAT or ID number",
			"the customer's name and address", "the customer's VAT or ID number", "the quantity and kind of goods or service", "the date of the supply or payment",
			"unit price without VAT and the taxable amount, with any discounts", "the VAT rate, or the legal basis for zero or no VAT", "the VAT amount", "the amount to pay, when it differs"},
		{"наименование на документа", "пореден десетразряден номер", "дата на издаване", "име и адрес на доставчика", "идентификационен номер на доставчика",
			"име и адрес на получателя", "идентификационен номер на получателя", "количество и вид на стоката или услугата", "дата на данъчното събитие или на плащането",
			"единична цена без данъка и данъчна основа, с отстъпките", "ставка на данъка или основание за нулева ставка или неначисляване", "размер на данъка", "сума за плащане, ако се различава"},
	},
	Sources: []Source{
		{Title: "Закон за данъка върху добавената стойност (ЗДДС), изм. ДВ бр. 115/2025", URL: zddsURL, InForce: "2026-01-01", Checked: "2026-10-26"},
		{Title: "PwC Worldwide Tax Summaries — Bulgaria, other taxes", URL: "https://taxsummaries.pwc.com/bulgaria/corporate/other-taxes", Checked: "2026-10-26"},
	},
	Checked: "2026-10-26",
}

// custom is any other country: its owner gives the rates; nothing is
// claimed about its law.
var custom = Pack{
	Country: "custom", EN: "Another country (your own rates)", BG: "Друга държава (ваши ставки)", Currency: "",
	Due: 0,
}

// Packs are the countries there are rules for.
func Packs() []Pack { return []Pack{bulgaria, custom} }

// PackFor is the pack of a country, with the owner's own rates for one
// without rules.
func PackFor(country string, ownRates []string, currency string) Pack {
	if strings.EqualFold(country, "bg") {
		return bulgaria
	}

	p := custom
	p.Currency = currency

	for _, r := range ownRates {
		r = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(r), "%"))
		if _, ok := new(big.Rat).SetString(r); ok {
			p.Rates = append(p.Rates, Rate{Value: r, EN: r + " %", BG: r + " %"})
		}
	}

	return p
}

// Kinds of invoice.
const (
	Sale     = "sale"     // issued: VAT charged
	Purchase = "purchase" // received: VAT that may be deducted
)

// Invoice is one invoice in the register.
type Invoice struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Number   string `json:"number"`
	Date     string `json:"date"` // YYYY-MM-DD
	Party    string `json:"party"`
	PartyVAT string `json:"party_vat,omitempty"`
	Net      string `json:"net"`
	Rate     string `json:"rate"`
	VAT      string `json:"vat"`
	Gross    string `json:"gross"`
	Currency string `json:"currency,omitempty"`

	// Reverse is reverse charge: the customer accounts for the VAT, and
	// the invoice shows neither rate nor amount.
	Reverse bool   `json:"reverse,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Finding is one thing a check found, with a key and its values so the
// page can say it in the reader's language.
type Finding struct {
	OK   bool              `json:"ok"`
	What string            `json:"what"`
	Key  string            `json:"key"`
	Args map[string]string `json:"args,omitempty"`
}

var findings = map[string]string{
	"vatMath":       "invoice {number}: {rate} % of {net} is {want}, but the invoice says {vat}",
	"grossMath":     "invoice {number}: {net} + {vat} is {want}, but the invoice says {gross}",
	"rateUnknown":   "invoice {number}: {rate} % is not a rate in {country}; the rates are {rates}",
	"zeroBasis":     "invoice {number}: a zero rate needs the legal basis written on the invoice",
	"numberForm":    "invoice {number}: an issued invoice's number does not have the form {country} requires",
	"vatID":         "invoice {number}: {id} does not look like a {country} VAT number",
	"noNumber":      "an invoice has no number",
	"noDate":        "invoice {number} has no date, or one that is not a date",
	"future":        "invoice {number} is dated {date}, in the future",
	"noParty":       "invoice {number} does not say who it is {with}",
	"duplicate":     "invoice number {number} is in the register {n} times",
	"gap":           "issued invoice numbers are missing between {from} and {to}: {missing}",
	"reverseVAT":    "invoice {number} is reverse charge but shows VAT of {vat}; under reverse charge the invoice shows no rate and no amount",
	"amount":        "invoice {number}: {field} {value} is not an amount",
	"invoiceOK":     "invoice {number}: the amounts add up and the rate is one of {country}'s",
	"periodOK":      "{n} invoices in {period}; every amount adds up",
	"threshold":     "sales in {year} so far: {sales} of the {threshold} registration threshold ({pct} %)",
	"overThreshold": "sales in {year} passed the {threshold} registration threshold on {date}, with invoice {number}",
}

func found(ok bool, key string, kv ...any) Finding {
	f := Finding{OK: ok, Key: key, Args: map[string]string{}}
	what := findings[key]

	for i := 0; i+1 < len(kv); i += 2 {
		name, value := fmt.Sprint(kv[i]), fmt.Sprint(kv[i+1])
		f.Args[name] = value
		what = strings.ReplaceAll(what, "{"+name+"}", value)
	}

	f.What = what

	return f
}

// FindingKeys are every kind of finding, for checking each is translated.
func FindingKeys() []string {
	out := make([]string, 0, len(findings))
	for k := range findings {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// Amount reads an amount however it is written, exactly. Empty is zero.
func Amount(s string) (*big.Rat, bool) {
	if strings.TrimSpace(s) == "" {
		return new(big.Rat), true
	}

	style := sheet.StyleOf(s)
	if style == sheet.Unknown {
		style = sheet.Comma // "12,50" reads as twelve and a half on an invoice
		if strings.Contains(s, ".") && !strings.Contains(s, ",") {
			style = sheet.Point
		}
	}

	return sheet.Number(s, style)
}

// Money writes an amount to the cent.
func Money(r *big.Rat) string { return r.FloatString(2) }

var cent = big.NewRat(1, 100)

func near(a, b *big.Rat) bool {
	d := new(big.Rat).Sub(a, b)

	return d.Abs(d).Cmp(cent) <= 0
}

// Normalise fills in what an invoice can be worked out to have — the VAT
// from the net and the rate, the gross from both — and writes its amounts
// to the cent. What it was given is kept; Check says whether it adds up.
func Normalise(inv Invoice) Invoice {
	inv.Number = strings.TrimSpace(inv.Number)
	inv.PartyVAT = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(inv.PartyVAT), " ", ""))
	inv.Rate = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(inv.Rate), "%"))
	inv.Rate = strings.TrimSpace(strings.Replace(inv.Rate, ",", ".", 1))

	if inv.Kind != Purchase {
		inv.Kind = Sale
	}

	net, okN := Amount(inv.Net)
	rate, okR := new(big.Rat).SetString(orZero(inv.Rate))

	if okN && okR && strings.TrimSpace(inv.VAT) == "" && !inv.Reverse {
		inv.VAT = Money(sheet.Round(new(big.Rat).Quo(new(big.Rat).Mul(net, rate), big.NewRat(100, 1)), 2))
	}

	if vat, okV := Amount(inv.VAT); okN && okV && strings.TrimSpace(inv.Gross) == "" {
		inv.Gross = Money(new(big.Rat).Add(net, vat))
	}

	for _, p := range []*string{&inv.Net, &inv.VAT, &inv.Gross} {
		if v, ok := Amount(*p); ok && strings.TrimSpace(*p) != "" {
			*p = Money(v)
		}
	}

	if inv.Reverse && strings.TrimSpace(inv.VAT) == "" {
		inv.VAT = "0.00"
	}

	return inv
}

func orZero(s string) string {
	if strings.TrimSpace(s) == "" {
		return "0"
	}

	return s
}

func label(inv Invoice) string {
	if inv.Number != "" {
		return inv.Number
	}

	return "(" + inv.Date + ")"
}

/*
 * Check is one invoice against the arithmetic and the country's rules:
 * the VAT is the rate of the net, to the cent; the gross is the net and the
 * VAT; the rate is one the country has; a zero rate says on what basis; an
 * issued invoice's number has the country's form; a VAT number looks like
 * one; the date is a date and not in the future; somebody is named. A
 * reverse-charge invoice shows no VAT.
 */
func Check(inv Invoice, p Pack, now time.Time) []Finding {
	var out []Finding

	n := label(inv)

	if inv.Number == "" {
		out = append(out, found(false, "noNumber"))
	}

	if d, err := time.Parse("2006-01-02", inv.Date); err != nil {
		out = append(out, found(false, "noDate", "number", n))
	} else if d.After(now.Add(24 * time.Hour)) {
		out = append(out, found(false, "future", "number", n, "date", inv.Date))
	}

	if strings.TrimSpace(inv.Party) == "" {
		with := "to"
		if inv.Kind == Purchase {
			with = "from"
		}

		out = append(out, found(false, "noParty", "number", n, "with", with))
	}

	amounts := map[string]*big.Rat{}

	for field, v := range map[string]string{"net": inv.Net, "VAT": inv.VAT, "gross": inv.Gross} {
		a, ok := Amount(v)
		if !ok {
			out = append(out, found(false, "amount", "number", n, "field", field, "value", v))

			continue
		}

		amounts[field] = a
	}

	rate, rateOK := new(big.Rat).SetString(orZero(inv.Rate))

	if inv.Reverse {
		if v := amounts["VAT"]; v != nil && v.Sign() != 0 {
			out = append(out, found(false, "reverseVAT", "number", n, "vat", Money(v)))
		}
	} else if rateOK && len(p.Rates) > 0 {
		known := false

		var rates []string

		for _, r := range p.Rates {
			rv, _ := new(big.Rat).SetString(r.Value)
			if rv != nil && rv.Cmp(rate) == 0 {
				known = true
			}

			rates = append(rates, r.Value+" %")
		}

		if !known {
			out = append(out, found(false, "rateUnknown", "number", n, "rate", inv.Rate, "country", p.EN, "rates", strings.Join(rates, ", ")))
		} else if rate.Sign() == 0 && inv.Kind == Sale && strings.TrimSpace(inv.Note) == "" {
			out = append(out, found(false, "zeroBasis", "number", n))
		}
	}

	if net, vat := amounts["net"], amounts["VAT"]; net != nil && vat != nil && rateOK && !inv.Reverse {
		want := sheet.Round(new(big.Rat).Quo(new(big.Rat).Mul(net, rate), big.NewRat(100, 1)), 2)
		if !near(want, vat) {
			out = append(out, found(false, "vatMath", "number", n, "rate", inv.Rate, "net", Money(net), "want", Money(want), "vat", Money(vat)))
		}
	}

	if net, vat, gross := amounts["net"], amounts["VAT"], amounts["gross"]; net != nil && vat != nil && gross != nil {
		want := new(big.Rat).Add(net, vat)
		if !near(want, gross) {
			out = append(out, found(false, "grossMath", "number", n, "net", Money(net), "vat", Money(vat), "want", Money(want), "gross", Money(gross)))
		}
	}

	if inv.Kind == Sale && p.Number != nil && inv.Number != "" && !p.Number.MatchString(inv.Number) {
		out = append(out, found(false, "numberForm", "number", n, "country", p.EN))
	}

	if id := inv.PartyVAT; id != "" && p.VATID != nil && strings.HasPrefix(id, strings.ToUpper(p.Country)) && !p.VATID.MatchString(id) {
		out = append(out, found(false, "vatID", "number", n, "id", id, "country", p.EN))
	}

	if len(out) == 0 {
		out = append(out, found(true, "invoiceOK", "number", n, "country", p.EN))
	}

	return out
}

/*
 * RegisterFindings are what only the whole register shows: a number used
 * twice among the issued invoices, and numbers missing from their sequence
 * (Bulgarian law wants issued invoices numbered in order, so a gap is
 * either a missing invoice or one that needs explaining).
 */
func RegisterFindings(list []Invoice) []Finding {
	var out []Finding

	count := map[string]int{}

	var numbers []int64

	for _, inv := range list {
		if inv.Kind != Sale || inv.Number == "" {
			continue
		}

		count[inv.Number]++

		if n, err := strconv.ParseInt(inv.Number, 10, 64); err == nil && count[inv.Number] == 1 {
			numbers = append(numbers, n)
		}
	}

	var dups []string
	for num, c := range count {
		if c > 1 {
			dups = append(dups, num)
		}
	}

	sort.Strings(dups)

	for _, d := range dups {
		out = append(out, found(false, "duplicate", "number", d, "n", count[d]))
	}

	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })

	for i := 1; i < len(numbers); i++ {
		gap := numbers[i] - numbers[i-1]
		if gap <= 1 || gap > 1000 {
			continue // a jump of over a thousand is another series
		}

		var missing []string

		for m := numbers[i-1] + 1; m < numbers[i] && len(missing) < 10; m++ {
			missing = append(missing, fmt.Sprintf("%010d", m))
		}

		if gap-1 > 10 {
			missing = append(missing, fmt.Sprintf("… %d in all", gap-1))
		}

		out = append(out, found(false, "gap", "from", fmt.Sprintf("%010d", numbers[i-1]), "to", fmt.Sprintf("%010d", numbers[i]),
			"missing", strings.Join(missing, ", ")))
	}

	return out
}

// Line is one rate's totals in a period.
type Line struct {
	Rate string `json:"rate"`
	Net  string `json:"net"`
	VAT  string `json:"vat"`
	N    int    `json:"n"`
}

// Period is a month's (or a quarter's, or a year's) VAT.
type Period struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Sales     []Line    `json:"sales"`
	Purchases []Line    `json:"purchases"`
	Reverse   Line      `json:"reverse"`
	Output    string    `json:"output"`
	Input     string    `json:"input"`
	Result    string    `json:"result"` // output − input: positive to pay, negative to deduct
	ToPay     bool      `json:"to_pay"`
	Due       string    `json:"due,omitempty"`
	DueRule   [2]string `json:"due_rule"`
	Findings  []Finding `json:"findings"`
	Invoices  int       `json:"invoices"`
}

// PeriodBounds is the first and last day of a period written "2026-09"
// (a month), "2026-Q3" (a quarter) or "2026" (a year).
func PeriodBounds(period string) (time.Time, time.Time, error) {
	period = strings.ToUpper(strings.TrimSpace(period))

	if t, err := time.Parse("2006-01", period); err == nil {
		return t, t.AddDate(0, 1, -1), nil
	}

	var y, q int
	if n, _ := fmt.Sscanf(period, "%d-Q%d", &y, &q); n == 2 && q >= 1 && q <= 4 {
		from := time.Date(y, time.Month(3*q-2), 1, 0, 0, 0, 0, time.UTC)

		return from, from.AddDate(0, 3, -1), nil
	}

	if t, err := time.Parse("2006", period); err == nil {
		return t, t.AddDate(1, 0, -1), nil
	}

	return time.Time{}, time.Time{}, fmt.Errorf("%q is not a period: write 2026-09, 2026-Q3 or 2026", period)
}

/*
 * Summarise works a period's VAT out: the VAT charged on sales and the VAT
 * on purchases, by rate, each added up exactly from the invoices; the
 * result to pay or to deduct; when the return and the payment are due
 * under the pack; and every invoice of the period checked again.
 */
func Summarise(list []Invoice, period string, p Pack, now time.Time) (Period, error) {
	from, to, err := PeriodBounds(period)
	if err != nil {
		return Period{}, err
	}

	out := Period{From: from.Format("2006-01-02"), To: to.Format("2006-01-02"), DueRule: p.DueRule}

	type sums struct {
		net, vat *big.Rat
		n        int
	}

	sales, purchases := map[string]*sums{}, map[string]*sums{}
	reverse := &sums{net: new(big.Rat), vat: new(big.Rat)}
	output, input := new(big.Rat), new(big.Rat)

	var in []Invoice

	for _, inv := range list {
		d, err := time.Parse("2006-01-02", inv.Date)
		if err != nil || d.Before(from) || d.After(to) {
			continue
		}

		in = append(in, inv)

		net, _ := Amount(inv.Net)
		vat, _ := Amount(inv.VAT)

		if net == nil {
			net = new(big.Rat)
		}

		if vat == nil {
			vat = new(big.Rat)
		}

		if inv.Reverse {
			reverse.net.Add(reverse.net, net)
			reverse.n++

			continue
		}

		book := sales
		total := output

		if inv.Kind == Purchase {
			book, total = purchases, input
		}

		s := book[inv.Rate]
		if s == nil {
			s = &sums{net: new(big.Rat), vat: new(big.Rat)}
			book[inv.Rate] = s
		}

		s.net.Add(s.net, net)
		s.vat.Add(s.vat, vat)
		s.n++
		total.Add(total, vat)
	}

	lines := func(m map[string]*sums) []Line {
		var out []Line

		for r, s := range m {
			out = append(out, Line{Rate: r, Net: Money(s.net), VAT: Money(s.vat), N: s.n})
		}

		sort.Slice(out, func(i, j int) bool {
			a, _ := strconv.ParseFloat(out[i].Rate, 64)
			b, _ := strconv.ParseFloat(out[j].Rate, 64)

			return a > b
		})

		return out
	}

	out.Sales, out.Purchases = lines(sales), lines(purchases)
	out.Reverse = Line{Net: Money(reverse.net), VAT: "0.00", N: reverse.n}
	out.Output, out.Input = Money(output), Money(input)

	result := new(big.Rat).Sub(output, input)
	out.Result, out.ToPay = Money(result), result.Sign() > 0
	out.Invoices = len(in)

	// The tax period is a month: a quarter or a year has no due date of
	// its own, only its months do.
	if _, err := time.Parse("2006-01", strings.TrimSpace(period)); err == nil && p.Due > 0 {
		out.Due = time.Date(to.Year(), to.Month()+1, p.Due, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}

	bad := 0

	for _, inv := range in {
		for _, f := range Check(inv, p, now) {
			if !f.OK {
				out.Findings = append(out.Findings, f)
				bad++
			}
		}
	}

	out.Findings = append(out.Findings, RegisterFindings(in)...)

	if bad == 0 && len(in) > 0 {
		out.Findings = append([]Finding{found(true, "periodOK", "n", len(in), "period", period)}, out.Findings...)
	}

	return out, nil
}

// Watch is the year's sales against the registration threshold.
type Watch struct {
	Year      int     `json:"year"`
	Sales     string  `json:"sales"`
	Threshold string  `json:"threshold"`
	Share     float64 `json:"share"`
	Finding   Finding `json:"finding"`
}

// ThresholdWatch adds up a calendar year's sales (without VAT) and says
// how near the registration threshold they are, and on which invoice they
// passed it. Nothing when the pack has no threshold.
func ThresholdWatch(list []Invoice, p Pack, year int) (Watch, bool) {
	limit, ok := new(big.Rat).SetString(orZero(p.Threshold))
	if !ok || limit.Sign() == 0 {
		return Watch{}, false
	}

	var sales []Invoice

	for _, inv := range list {
		if inv.Kind == Sale && strings.HasPrefix(inv.Date, strconv.Itoa(year)+"-") {
			sales = append(sales, inv)
		}
	}

	sort.SliceStable(sales, func(i, j int) bool { return sales[i].Date < sales[j].Date })

	total := new(big.Rat)

	w := Watch{Year: year, Threshold: Money(limit)}

	for _, inv := range sales {
		net, ok := Amount(inv.Net)
		if !ok {
			continue
		}

		before := total.Cmp(limit) > 0

		total.Add(total, net)

		if !before && total.Cmp(limit) > 0 {
			w.Finding = found(false, "overThreshold", "year", year, "threshold", Money(limit)+" "+p.Currency, "date", inv.Date, "number", label(inv))
		}
	}

	w.Sales = Money(total)
	share, _ := new(big.Rat).Quo(total, limit).Float64()
	w.Share = share * 100

	if w.Finding.Key == "" {
		w.Finding = found(true, "threshold", "year", year, "sales", Money(total)+" "+p.Currency, "threshold", Money(limit)+" "+p.Currency,
			"pct", strconv.FormatFloat(float64(int(w.Share*10))/10, 'f', -1, 64))
	}

	return w, true
}
