/*
 * Tables: a table looked over, a question answered from it with every
 * total checked a second way, and the result saved as a new workbook with
 * its checks (server/tables.go). Cells are somebody's data, so everything
 * is drawn with textContent.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    let looked = null;
    let done = null;

    async function send(path, body) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: JSON.stringify(body || {}),
        });
        const data = await res.json().catch(() => ({}));

        if (!res.ok) {
            const err = new Error(data.error || `${res.status}`);
            err.status = res.status;
            throw err;
        }

        return data.data !== undefined ? data.data : data;
    }

    function say(id, text, bad) {
        const p = el(id);
        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    const failedText = (err) => (err.status === 503 ? T('tab.noModel') : T('writing.failed', { why: err.message }));

    // A number cell is right-aligned, as numbers are read.
    const isNumber = (s) => /^-?[\d\s.,]+$/.test(String(s).trim()) && /\d/.test(s);

    function drawTable(table, header, rows) {
        table.replaceChildren();

        const thead = document.createElement('thead');
        const tr = document.createElement('tr');

        for (const h of header) {
            const th = document.createElement('th');
            th.scope = 'col';
            th.textContent = h;
            tr.append(th);
        }

        thead.append(tr);

        const tbody = document.createElement('tbody');

        for (const r of rows) {
            const row = document.createElement('tr');

            for (const c of r) {
                const td = document.createElement('td');
                td.textContent = c;
                if (isNumber(c)) td.className = 'is-num';
                row.append(td);
            }

            tbody.append(row);
        }

        table.append(thead, tbody);
    }

    function drawFindings(ul, findings) {
        ul.replaceChildren();

        for (const f of findings || []) {
            const li = document.createElement('li');
            li.className = `writing-item research-claim ${f.ok ? 'is-ok' : 'is-flagged'}`;

            const mark = document.createElement('strong');
            mark.textContent = f.ok ? T('tab.ok') : T('tab.lookAt');

            const what = document.createElement('span');
            what.className = 'research-claim-text';
            // Said in the reader's language when the page knows the kind.
            const key = f.key ? `tab.f.${f.key}` : '';
            const said = key ? T(key, f.args || {}) : '';
            what.textContent = said && said !== key ? said : f.what;

            li.append(mark, what);
            ul.append(li);
        }
    }

    const kindName = (k) => T(`tab.kind.${k}`);

    function showLooked(data, quiet) {
        looked = data;

        const s = data.summary;
        el('tab-summary-title').textContent = s.name;
        el('tab-summary-meta').textContent = T('tab.rows', { n: s.rows });

        drawTable(el('tab-columns'),
            [T('tab.col'), T('tab.kind'), T('tab.filled'), T('tab.blank'), T('tab.sum'), T('tab.min'), T('tab.max'), T('tab.mean')],
            s.columns.map((c) => [c.name, kindName(c.kind), String(c.filled), String(c.blank), c.sum || '', c.min || '', c.max || '', c.mean || '']));

        drawFindings(el('tab-findings'), s.findings);

        const select = el('tab-sheet');
        const sheets = data.sheets || [];

        if (!quiet) {
            const current = select.value;
            select.replaceChildren();

            for (const name of sheets) {
                const o = document.createElement('option');
                o.value = name;
                o.textContent = name;
                select.append(o);
            }

            select.value = sheets.includes(current) ? current : s.name;
        }

        el('tab-sheet-field').hidden = sheets.length < 2;
        el('tab-summary-card').hidden = false;
        el('tab-ask-card').hidden = false;

        if (!quiet && !el('tab-out').value) {
            const path = el('tab-path').value.trim();
            el('tab-out').value = path.replace(/\.[^./\\]+$/, '') + ` — ${T('tab.resultName')}.xlsx`;
        }
    }

    async function look() {
        const path = el('tab-path').value.trim();

        if (!path) { el('tab-path').focus(); return; }

        el('tab-look').disabled = true;
        say('tab-said', T('tab.reading'));

        try {
            const data = await send('/api/tables/look', { path, sheet: el('tab-sheet-field').hidden ? '' : el('tab-sheet').value });
            say('tab-said', '');
            el('tab-result').hidden = true;
            done = null;
            showLooked(data);
        } catch (err) {
            say('tab-said', failedText(err), true);
        } finally {
            el('tab-look').disabled = false;
        }
    }

    function showDone(data) {
        done = data;

        el('tab-explain').textContent = data.recipe && data.recipe.explain ? data.recipe.explain : '';
        drawTable(el('tab-rows'), data.header || [], data.rows || []);

        const f = data.forecast || [];
        el('tab-forecast-box').hidden = f.length < 2;
        if (f.length >= 2) drawTable(el('tab-forecast'), f[0], f.slice(1));

        drawFindings(el('tab-checks'), data.checks);
        el('tab-recipe').textContent = JSON.stringify(data.recipe || {}, null, 2);
        el('tab-result').hidden = false;
    }

    async function run() {
        const question = el('tab-question').value.trim();

        if (!question) { el('tab-question').focus(); return; }

        el('tab-run').disabled = true;
        say('tab-run-said', T('tab.working'));

        try {
            const data = await send('/api/tables/calculate', {
                path: el('tab-path').value.trim(),
                sheet: el('tab-sheet-field').hidden ? '' : el('tab-sheet').value,
                question,
            });

            const bad = (data.checks || []).filter((c) => !c.ok).length;
            say('tab-run-said', bad ? T('tab.someToLook', { n: bad }) : T('tab.allChecked'));
            say('tab-save-said', '');
            showDone(data);
        } catch (err) {
            say('tab-run-said', failedText(err), true);
        } finally {
            el('tab-run').disabled = false;
        }
    }

    async function save() {
        if (!done) return;

        const out = el('tab-out').value.trim();
        if (!out) { el('tab-out').focus(); return; }

        try {
            const data = await send('/api/tables/save', {
                path: el('tab-path').value.trim(),
                sheet: el('tab-sheet-field').hidden ? '' : el('tab-sheet').value,
                recipe: done.recipe,
                out,
            });

            say('tab-save-said', T('tab.saved', { path: data.saved }));
        } catch (err) {
            say('tab-save-said', err.status === 409 ? T('tab.exists') : T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('tab-look')) return;

        el('tab-look').addEventListener('click', look);
        el('tab-sheet').addEventListener('change', look);
        el('tab-run').addEventListener('click', run);
        el('tab-save').addEventListener('click', save);

        for (const [input, fn] of [['tab-path', look], ['tab-question', run]]) {
            el(input).addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    fn();
                }
            });
        }

        window.addEventListener('pn-language', () => {
            if (looked) showLooked(looked, true);
            if (done) showDone(done);
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
