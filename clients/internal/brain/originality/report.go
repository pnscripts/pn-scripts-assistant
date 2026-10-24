package originality

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Finding is one row of an audit: item | aspect | finding | risk | action.
type Finding struct {
	Item    string `json:"item"`
	Aspect  string `json:"aspect"`
	Finding string `json:"finding"`
	Risk    Risk   `json:"risk"`
	Action  string `json:"action"`
	// Fixed is what was changed, when the assistant made the item and
	// changed it itself.
	Fixed string `json:"fixed,omitempty"`
}

var riskOrder = map[Risk]int{High: 0, Medium: 1, Low: 2, None: 3}

// Sort puts the findings that need action first.
func Sort(f []Finding) {
	sort.SliceStable(f, func(i, j int) bool {
		if riskOrder[f[i].Risk] != riskOrder[f[j].Risk] {
			return riskOrder[f[i].Risk] < riskOrder[f[j].Risk]
		}

		return f[i].Item < f[j].Item
	})
}

// Summary is one line: how many findings of each risk.
func Summary(f []Finding) string {
	count := map[Risk]int{}
	for _, x := range f {
		count[x.Risk]++
	}

	return fmt.Sprintf("%d high, %d medium, %d low, %d clear", count[High], count[Medium], count[Low], count[None])
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "|", "\\|"), "\n", " ")
}

// Markdown is the report as a table people read.
func Markdown(title string, f []Finding, when time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n%s · %s\n\n", title, when.Format("2006-01-02 15:04"), Summary(f))
	b.WriteString("| Item | Aspect | Finding | Risk | Action |\n|---|---|---|---|---|\n")

	for _, x := range f {
		action := x.Action
		if x.Fixed != "" {
			action = "Fixed: " + x.Fixed
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", cell(x.Item), cell(x.Aspect), cell(x.Finding), x.Risk, cell(action))
	}

	b.WriteString("\nMeasured by comparison with public sources and the references given; " +
		"it is not legal advice. High means change it or have it cleared before publishing.\n")

	return b.String()
}

/*
 * Write saves the report as Markdown and CSV in dir, under a new name each
 * time — a report is a record, and a second audit must not replace the first.
 * Returns both paths.
 */
func Write(dir, title string, f []Finding, when time.Time) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}

	Sort(f)

	base := filepath.Join(dir, "originality-audit-"+when.Format("20060102-150405"))

	for i := 2; ; i++ {
		if _, err := os.Stat(base + ".md"); os.IsNotExist(err) {
			break
		}

		base = filepath.Join(dir, fmt.Sprintf("originality-audit-%s-%d", when.Format("20060102-150405"), i))
	}

	md, sheet := base+".md", base+".csv"

	if err := os.WriteFile(md, []byte(Markdown(title, f, when)), 0o644); err != nil {
		return "", "", err
	}

	out, err := os.Create(sheet)
	if err != nil {
		return "", "", err
	}
	defer out.Close()

	w := csv.NewWriter(out)
	_ = w.Write([]string{"item", "aspect", "finding", "risk", "action", "fixed"})

	for _, x := range f {
		_ = w.Write([]string{x.Item, x.Aspect, x.Finding, string(x.Risk), x.Action, x.Fixed})
	}

	w.Flush()

	return md, sheet, w.Error()
}
