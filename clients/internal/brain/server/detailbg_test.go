package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/progress"
)

/*
 * The lines under a step are said in the page's language.
 *
 * Seen 15.11 on a Bulgarian page: under "Записване на notes.txt" the line
 * "Wrote 22 bytes to ... in 0ms", the tool's own English for the model. A
 * line that carries its Bulgarian (detail_bg, side by side with detail)
 * is said in it; one that does not is said as before, by its shape. Both
 * Doing now (feed.js) and the steps under an answer (console.js) draw
 * through the one helper, which this runs in node on the real /api/steps.
 */
func TestTheLinesUnderAStepAreSaidInThePagesLanguage(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, _, _ := newServer(t)

	progress.Begin()
	progress.SetToolIn("write_file", "Write zz-detail.txt", "Записване на zz-detail.txt")

	mark := progress.Mark()

	progress.DetailOn(mark, "nothing back in 1.5s")
	progress.DetailOn(mark, "1 word in 80ms")
	progress.DetailOnIn(mark, "Wrote 22 bytes to zz-detail.txt in 3ms", "резултат от 38 знака за 3 мс")
	progress.Done()

	steps := fetched(t, ts.URL+"/api/steps")

	for _, js := range []string{"feed.js", "console.js"} {
		if !strings.Contains(readAsset(t, filepath.Join("js", js)), "window.pnDetailLine(") {
			t.Errorf("%s draws the lines under a step without pnDetailLine", js)
		}
	}

	en := detailLinesSaid(t, "en-GB", steps)
	bg := detailLinesSaid(t, "bg-BG", steps)

	if strings.Join(en, " | ") != "nothing back in 1.5s | 1 word in 80ms | Wrote 22 bytes to zz-detail.txt in 3ms" {
		t.Errorf("in English: %q", en)
	}

	if strings.Join(bg, " | ") != "без резултат за 1,5 с | 1 дума за 80 мс | резултат от 38 знака за 3 мс" {
		t.Errorf("in Bulgarian: %q", bg)
	}
}

// detailLinesSaid runs i18n.js and status.js in node and says every line
// under the step written for this test, as the page would.
func detailLinesSaid(t *testing.T, language string, steps []byte) []string {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "status.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	os.WriteFile(filepath.Join(dir, "steps.json"), steps, 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];
Object.defineProperty(globalThis, 'navigator', { value: { language: process.argv[3] }, configurable: true });
globalThis.window = globalThis;
window.dispatchEvent = () => true;
window.addEventListener = () => {};
globalThis.CustomEvent = function () {};
globalThis.document = { readyState: 'complete', querySelectorAll: () => [], querySelector: () => null,
    getElementById: () => null, documentElement: { dataset: {}, style: { setProperty() {} } },
    addEventListener: () => {} };
globalThis.setInterval = () => 0;
globalThis.fetch = () => Promise.reject(new Error('no server here'));
const read = (name) => fs.readFileSync(path.join(dir, name), 'utf8');
(0, eval)(read('i18n.js'));
(0, eval)(read('status.js'));
const body = JSON.parse(read('steps.json'));
const steps = (body.data !== undefined ? body.data : body).steps;
const step = steps.find((s) => s.note === 'Write zz-detail.txt');
process.stdout.write(JSON.stringify((step.detail || []).map((line, i) => window.pnDetailLine(step, i))));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said []string
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}
