/*
 * Invoices: the invoice register, a month's VAT and when it is due, the
 * registration threshold, and the rules they rest on with their sources and
 * the date they were checked (server/books.go). Information, never a
 * filing. Invoices are somebody's business and a document read by a model
 * is a stranger's text, so everything is drawn with textContent.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => document.documentElement.lang === 'bg';

    let page = null;
    let summary = null;

    async function get(path) {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `${res.status}`);

        return body.data !== undefined ? body.data : body;
    }

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

    // An amount as the reader writes it: 1650.00 in English, 1650,00 in
    // Bulgarian. Only the writing changes; what is kept is the same.
    const money = (v) => (bg() && /^-?\d+\.\d{1,4}$/.test(String(v)) ? String(v).replace('.', ',') : String(v ?? ''));

    // A finding in the reader's language when the page knows its kind.
    function findingText(f) {
        const key = f.key ? `books.f.${f.key}` : '';
        const args = {};
        for (const [k, v] of Object.entries(f.args || {})) args[k] = money(v);
        const said = key ? T(key, args) : '';

        return said && said !== key ? said : f.what;
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
            what.textContent = findingText(f);

            li.append(mark, what);
            ul.append(li);
        }
    }

    function table(t, header, rows, numeric) {
        t.replaceChildren();

        const head = document.createElement('tr');
        header.forEach((h, i) => {
            const th = document.createElement('th');
            th.scope = 'col';
            th.textContent = h;
            if (numeric.includes(i)) th.className = 'is-num';
            head.append(th);
        });

        const thead = document.createElement('thead');
        thead.append(head);

        const tbody = document.createElement('tbody');

        for (const r of rows) {
            const tr = document.createElement('tr');

            r.cells.forEach((c, i) => {
                const td = document.createElement('td');
                if (c instanceof Node) td.append(c);
                else td.textContent = numeric.includes(i) ? money(c) : c;
                if (numeric.includes(i)) td.className = 'is-num';
                tr.append(td);
            });

            tbody.append(tr);
        }

        t.append(thead, tbody);
    }

    function openWhere(where) {
        const here = ['127.0.0.1', 'localhost', '[::1]'].includes(location.hostname);

        if (!here) {
            window.open(where, '_blank', 'noopener,noreferrer');
            return;
        }

        fetch('/api/open-source', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ where }),
        }).catch(() => {});
    }

    /* ---------- The rules ---------- */

    function drawRules(pack) {
        const box = el('books-rules');
        box.replaceChildren();

        const i = bg() ? 1 : 0;

        const add = (tag, text, cls) => {
            const n = document.createElement(tag);
            n.textContent = text;
            if (cls) n.className = cls;
            box.append(n);
            return n;
        };

        if (!pack.checked) {
            add('p', T('books.ownRules'), 'note');
            return;
        }

        add('p', T('books.checked', { date: pack.checked }), 'note');

        const ul = document.createElement('ul');
        ul.className = 'books-rule-list';

        for (const r of pack.rates || []) {
            const li = document.createElement('li');
            const b = document.createElement('strong');
            b.textContent = `${r.value} % — ${i ? r.bg : r.en}`;
            li.append(b, document.createTextNode(`: ${r.covers[i]} (${r.rule})`));
            ul.append(li);
        }

        for (const rule of [pack.threshold_rule, pack.due_rule, pack.invoice_rule, pack.number_rule]) {
            if (rule && rule[i]) {
                const li = document.createElement('li');
                li.textContent = rule[i];
                ul.append(li);
            }
        }

        box.append(ul);

        if (((pack.fields || [])[i] || []).length) {
            const d = document.createElement('details');
            const s = document.createElement('summary');
            s.textContent = T('books.fields');
            const ol = document.createElement('ol');

            for (const f of pack.fields[i]) {
                const li = document.createElement('li');
                li.textContent = f;
                ol.append(li);
            }

            d.append(s, ol);
            box.append(d);
        }

        add('h3', T('books.sources'), 'tab-sub');

        const src = document.createElement('ul');
        src.className = 'books-rule-list';

        for (const s of pack.sources || []) {
            const li = document.createElement('li');
            const a = document.createElement('a');
            a.href = s.url;
            a.rel = 'noopener noreferrer';
            a.textContent = s.title;
            a.addEventListener('click', (e) => { e.preventDefault(); openWhere(s.url); });

            li.append(a, document.createTextNode(` — ${T('books.sourceChecked', { date: s.checked })}${s.in_force ? `, ${T('books.inForce', { date: s.in_force })}` : ''}`));
            src.append(li);
        }

        box.append(src);
    }

    function drawSettings() {
        const s = page.settings;
        const select = el('books-country');

        select.replaceChildren();

        for (const p of page.packs) {
            const o = document.createElement('option');
            o.value = p.country;
            o.textContent = bg() ? p.bg : p.en;
            select.append(o);
        }

        select.value = s.country;
        el('books-own-rates').value = s.rates || '';
        el('books-currency').value = s.currency || '';
        showOwn();

        const rate = el('books-rate');
        const current = rate.value;
        rate.replaceChildren();

        for (const r of page.pack.rates || []) {
            const o = document.createElement('option');
            o.value = r.value;
            o.textContent = `${r.value} %`;
            rate.append(o);
        }

        if (current) rate.value = current;

        drawRules(page.pack);
    }

    function showOwn() {
        const own = el('books-country').value === 'custom';
        el('books-own-rates-field').hidden = !own;
        el('books-currency-field').hidden = !own;
    }

    /* ---------- The register ---------- */

    function drawRegister() {
        const list = page.invoices || [];
        const rows = list.slice().reverse().map((inv) => {
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'model-action quiet';
            remove.textContent = T('study.remove');
            remove.addEventListener('click', async () => {
                if (!remove.classList.contains('danger')) {
                    remove.classList.add('danger');
                    remove.textContent = T('gl.sure');
                    setTimeout(() => { remove.classList.remove('danger'); remove.textContent = T('study.remove'); }, 4000);
                    return;
                }

                try {
                    await send(`/api/books/invoices/${inv.id}/remove`);
                    await load();
                } catch (err) {
                    say('books-add-said', T('writing.failed', { why: err.message }), true);
                }
            });

            return {
                cells: [inv.date, T(`books.${inv.kind}Short`), inv.number, inv.party,
                    inv.net, inv.reverse ? T('books.reverseShort') : `${inv.rate} %`, inv.vat, inv.gross, remove],
            };
        });

        table(el('books-register'),
            [T('books.date'), T('books.kind'), T('books.number'), T('books.party'), T('books.net'), T('books.rate'), T('books.vat'), T('books.gross'), ''],
            rows, [4, 6, 7]);

        el('books-empty').hidden = list.length > 0;
        drawFindings(el('books-register-findings'), page.findings || []);
    }

    async function load() {
        try {
            page = await get('/api/books');
            drawSettings();
            drawRegister();
        } catch (err) {
            say('books-said', T('writing.failed', { why: err.message }), true);
        }
    }

    /* ---------- A period's VAT ---------- */

    function drawSummary(s) {
        summary = s;

        const lines = [];
        for (const l of s.sales || []) lines.push({ cells: [T('books.salesAt', { rate: l.rate }), String(l.n), l.net, l.vat] });
        if (s.reverse && s.reverse.n) lines.push({ cells: [T('books.reverseLine'), String(s.reverse.n), s.reverse.net, '—'] });
        for (const l of s.purchases || []) lines.push({ cells: [T('books.purchasesAt', { rate: l.rate }), String(l.n), l.net, l.vat] });

        table(el('books-lines'), [T('books.line'), T('books.count'), T('books.net'), T('books.vat')], lines, [1, 2, 3]);

        const amount = String(s.result || '0').replace(/^-/, '');
        el('books-total').textContent = T(s.to_pay ? 'books.toPay' : 'books.toDeduct', {
            output: money(s.output), input: money(s.input), result: money(amount), currency: s.pack.currency || '',
        });

        const i = bg() ? 1 : 0;
        el('books-due').textContent = s.due ? T('books.due', { date: s.due, rule: s.due_rule[i] }) : (s.pack.checked ? T('books.noDueForSpan') : '');

        const w = s.watch;
        el('books-watch').hidden = !w;

        if (w) {
            el('books-meter').style.width = `${Math.min(100, w.share)}%`;
            el('books-meter').parentElement.classList.toggle('is-over', w.share > 100);
            el('books-watch-text').textContent = findingText(w.finding);
        }

        drawFindings(el('books-findings'), s.findings);
        el('books-result').hidden = false;
    }

    async function sum() {
        const period = el('books-period').value.trim();

        if (!period) { el('books-period').focus(); return; }

        try {
            say('books-said', '');
            drawSummary(await send('/api/books/summary', { period }));
        } catch (err) {
            say('books-said', T('writing.failed', { why: err.message }), true);
        }
    }

    /* ---------- Adding an invoice ---------- */

    function formInvoice() {
        return {
            kind: el('books-kind').value,
            number: el('books-number').value.trim(),
            date: el('books-date').value,
            party: el('books-party').value.trim(),
            party_vat: el('books-party-vat').value.trim(),
            net: el('books-net').value.trim(),
            rate: el('books-rate').value,
            vat: el('books-vat').value.trim(),
            gross: el('books-gross').value.trim(),
            reverse: el('books-reverse').checked,
            note: el('books-note').value.trim(),
        };
    }

    function fill(inv) {
        el('books-number').value = inv.number || '';
        el('books-date').value = inv.date || '';
        el('books-party').value = inv.party || '';
        el('books-party-vat').value = inv.party_vat || '';
        el('books-net').value = inv.net || '';
        el('books-vat').value = inv.vat || '';
        el('books-gross').value = inv.gross || '';
        el('books-note').value = inv.note || '';
        el('books-reverse').checked = !!inv.reverse;

        if (inv.rate && [...el('books-rate').options].some((o) => o.value === inv.rate)) el('books-rate').value = inv.rate;
    }

    async function record(keep) {
        const inv = formInvoice();

        if (!inv.date) { el('books-date').focus(); return; }
        if (!inv.net && !inv.gross) { el('books-net').focus(); return; }

        try {
            const r = await send('/api/books/invoices', { ...inv, keep });

            fill(r.invoice);
            drawFindings(el('books-add-findings'), r.findings);
            say('books-add-said', keep ? T('books.added', { n: r.invoice.number || r.invoice.date }) : T('books.checkedOnly'));

            if (keep) await load();
        } catch (err) {
            say('books-add-said', err.status === 409 ? T('books.twice') : T('writing.failed', { why: err.message }), true);
        }
    }

    async function read() {
        const path = el('books-doc').value.trim();

        if (!path) { el('books-doc').focus(); return; }

        el('books-read').disabled = true;
        say('books-add-said', T('books.reading'));

        try {
            const r = await send('/api/books/read', { path, kind: el('books-kind').value });

            fill(r.invoice);
            drawFindings(el('books-add-findings'), r.findings);
            say('books-add-said', T('books.readDone'));
        } catch (err) {
            say('books-add-said', err.status === 503 ? T('books.noModel') : T('writing.failed', { why: err.message }), true);
        } finally {
            el('books-read').disabled = false;
        }
    }

    async function importTable() {
        const path = el('books-import-path').value.trim();

        if (!path) { el('books-import-path').focus(); return; }

        try {
            const r = await send('/api/books/import', { path, kind: el('books-import-kind').value });

            say('books-import-said', T('books.imported', { n: r.kept })
                + ((r.skipped || []).length ? ` ${T('books.skipped', { rows: r.skipped.join(', ') })}` : ''));
            drawFindings(el('books-import-findings'), r.findings);
            await load();
        } catch (err) {
            say('books-import-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function save() {
        try {
            await send('/api/books/settings', {
                country: el('books-country').value,
                rates: el('books-own-rates').value.trim(),
                currency: el('books-currency').value.trim().toUpperCase(),
            });

            await load();
        } catch (err) {
            say('books-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function lastMonth() {
        const d = new Date();
        d.setDate(1);
        d.setMonth(d.getMonth() - 1);

        return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
    }

    function wire() {
        if (!el('books-register')) return;

        el('books-period').value = lastMonth();
        el('books-sum').addEventListener('click', sum);
        el('books-add').addEventListener('click', () => record(true));
        el('books-check').addEventListener('click', () => record(false));
        el('books-read').addEventListener('click', read);
        el('books-import').addEventListener('click', importTable);
        el('books-save').addEventListener('click', save);
        el('books-country').addEventListener('change', showOwn);
        el('books-period').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                sum();
            }
        });

        for (const item of document.querySelectorAll('.nav-item[data-view="invoices"]')) {
            item.addEventListener('click', load);
        }

        window.addEventListener('pn-language', () => {
            if (page) { drawSettings(); drawRegister(); }
            if (summary) drawSummary(summary);
        });

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
