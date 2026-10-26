/*
 * Research: a question looked into with sources, the reports kept, and a
 * text checked against the sources it cites (server/research.go).
 *
 * A report is a model's words built from web pages, so it is drawn with
 * textContent: the small Markdown reader below makes elements, never HTML.
 * A citation [n] becomes a link to source n in the list; a source opens in
 * the system's own browser, as every other source does.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    let lastList = null;
    let shown = null; // { report, sources, claims }

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

    function failed(id, err) {
        const key = { 403: 'research.noWeb', 503: 'research.noModel', 404: 'research.nothing' }[err.status];
        say(id, key ? T(key) : T('writing.failed', { why: err.message }), true);
    }

    function openWhere(where) {
        const here = ['127.0.0.1', 'localhost', '[::1]'].includes(location.hostname);

        if (!/^https?:\/\//i.test(where)) return;

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

    /* ---------- A report, as elements ---------- */

    // inline puts **bold**, `code`, [text](https://…) and [n] citations into
    // a parent as elements.
    function inline(parent, text) {
        // A citation sits against the word before it, as a footnote does.
        const parts = String(text).replace(/[ \u00a0]+(\[\d{1,3}\])/g, '$1').split(/(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]*\]\(https?:\/\/[^)\s]+\)|\[\d{1,3}\])/g);

        for (const part of parts) {
            if (!part) continue;

            const link = /^\[([^\]]*)\]\((https?:\/\/[^)\s]+)\)$/.exec(part);
            const cite = /^\[(\d{1,3})\]$/.exec(part);

            if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
                const b = document.createElement('strong');
                b.textContent = part.slice(2, -2);
                parent.append(b);
            } else if (part.startsWith('`') && part.endsWith('`') && part.length > 2) {
                const c = document.createElement('code');
                c.textContent = part.slice(1, -1);
                parent.append(c);
            } else if (link) {
                const a = document.createElement('a');
                a.href = link[2];
                a.rel = 'noopener noreferrer';
                a.textContent = link[1] || link[2];
                a.addEventListener('click', (e) => { e.preventDefault(); openWhere(link[2]); });
                parent.append(a);
            } else if (cite) {
                const a = document.createElement('a');
                a.className = 'research-cite';
                a.href = `#research-src-${cite[1]}`;
                a.textContent = cite[1];
                a.setAttribute('aria-label', T('research.sourceN', { n: cite[1] }));
                a.addEventListener('click', (e) => {
                    e.preventDefault();
                    const target = el(`research-src-${cite[1]}`);
                    if (target) {
                        target.scrollIntoView({ behavior: 'smooth', block: 'center' });
                        target.classList.add('is-lit');
                        setTimeout(() => target.classList.remove('is-lit'), 1600);
                    }
                });
                const sup = document.createElement('sup');
                sup.append(a);
                parent.append(sup);
            } else {
                parent.append(document.createTextNode(part.replace(/(^|[^*])\*([^*]+)\*/g, '$1$2').replace(/^_(.*)_$/, '$1')));
            }
        }
    }

    // draw reads the report's Markdown into box; the first heading is the
    // card's title and is given back. Items of the numbered list after the
    // last heading are the sources and get an id each.
    function draw(box, text) {
        box.replaceChildren();

        let title = '';
        let list = null;
        let para = null;
        let inSources = false;

        const close = () => { list = null; para = null; };

        for (const raw of String(text).split('\n')) {
            const line = raw.replace(/\s+$/, '');

            if (!line.trim()) { close(); continue; }

            const heading = /^(#{1,4})\s+(.*)$/.exec(line);
            if (heading) {
                close();
                const words = heading[2].replace(/[*#:]+$/g, '').trim();

                if (heading[1].length === 1 && !title) { title = words; continue; }

                inSources = /^(sources|източници)$/i.test(words);

                const h = document.createElement('h3');
                h.textContent = words;
                box.append(h);
                continue;
            }

            if (/^_.*_$/.test(line.trim()) && !box.childElementCount) {
                const meta = document.createElement('p');
                meta.className = 'research-line';
                meta.textContent = line.trim().slice(1, -1);
                box.append(meta);
                continue;
            }

            const item = /^\s*(?:[-*•]|(\d+)[.)])\s+(.*)$/.exec(line);
            if (item) {
                para = null;
                const kind = item[1] ? 'OL' : 'UL';

                if (!list || list.tagName !== kind) {
                    list = document.createElement(kind.toLowerCase());
                    if (item[1] && item[1] !== '1') list.start = Number(item[1]);
                    if (inSources) list.className = 'research-sources';
                    box.append(list);
                }

                const li = document.createElement('li');
                if (inSources && item[1]) li.id = `research-src-${item[1]}`;
                inline(li, item[2]);
                list.append(li);
                continue;
            }

            list = null;

            if (!para) {
                para = document.createElement('p');
                box.append(para);
            } else {
                para.append(' ');
            }

            inline(para, line.trim());
        }

        return title;
    }

    function chip(text, cls) {
        const c = document.createElement('span');
        c.className = `plan-chip ${cls || ''}`.trim();
        c.textContent = text;
        return c;
    }

    function show(data, quiet) {
        shown = data;

        const r = data.report;
        const card = el('research-report-card');
        const title = draw(el('research-report-text'), r.report || '');

        el('research-report-title').textContent = title || r.question;

        const meta = [T(`research.depthName.${r.depth}`)];
        if (r.model) meta.push(T('study.by', { model: r.model }));
        el('research-report-meta').textContent = meta.join(' · ');

        const sum = el('research-summary');
        sum.replaceChildren(
            chip(T('research.nSources', { n: r.n_sources })),
            chip(T('research.supported', { n: r.n_claims - r.n_flagged, of: r.n_claims }), 'is-ok'),
        );
        if (r.n_flagged) sum.append(chip(T('research.flagged', { n: r.n_flagged }), 'is-warn'));

        el('research-download').href = `/api/research/${r.id}/download`;
        card.hidden = false;
        if (!quiet) card.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    async function open(id) {
        try {
            show(await get(`/api/research/${id}`));
        } catch (err) {
            say('research-said', T('writing.failed', { why: err.message }), true);
        }
    }

    /* ---------- Running research ---------- */

    let ticking = null;

    function busy(on) {
        el('research-run').disabled = on;
        el('research-question').disabled = on;

        clearInterval(ticking);
        ticking = null;

        if (on) {
            const started = Date.now();
            const tick = () => say('research-said', T('research.working', { s: Math.round((Date.now() - started) / 1000) }));
            tick();
            ticking = setInterval(tick, 1000);
        }
    }

    async function run() {
        const question = el('research-question').value.trim();

        if (question.length < 3) {
            el('research-question').focus();
            return;
        }

        busy(true);

        try {
            const data = await send('/api/research', {
                question,
                depth: el('research-depth').value,
                language: el('research-language').value,
            });

            busy(false);
            say('research-said', T('research.done', { n: data.sources.length }));
            show(data);
            await loadList();
        } catch (err) {
            busy(false);
            failed('research-said', err);
        }
    }

    /* ---------- Checking a text ---------- */

    async function check() {
        const text = el('research-check-text').value.trim();
        const sources = el('research-check-sources').value.split('\n').map((s) => s.trim()).filter(Boolean);
        const out = el('research-check-out');

        out.replaceChildren();

        if (!text) { el('research-check-text').focus(); return; }
        if (!sources.length) { el('research-check-sources').focus(); return; }

        el('research-check-run').disabled = true;
        say('research-check-said', T('research.checking'));

        try {
            const data = await send('/api/research/check', { text, sources });
            const s = data.summary;

            say('research-check-said', T('research.checked', { n: s.claims, ok: s.supported, bad: s.flagged })
                + (s.removed ? ` ${T('research.removed', { n: s.removed })}` : ''));

            for (const u of data.unread || []) {
                const li = document.createElement('li');
                li.className = 'writing-item research-claim is-flagged';
                li.textContent = T('research.unread', { what: u });
                out.append(li);
            }

            for (const c of data.claims || []) {
                const flagged = c.verdict ? c.verdict !== 'confirmed' : c.status !== 'supported';
                const li = document.createElement('li');
                li.className = `writing-item research-claim ${flagged ? 'is-flagged' : 'is-ok'}`;

                const mark = document.createElement('strong');
                mark.textContent = flagged ? T('research.markCheck') : T('research.markOk');

                const said = document.createElement('span');
                said.className = 'research-claim-text';
                said.textContent = c.text;

                li.append(mark, said);

                const why = c.verdict ? T(`research.verdict.${c.verdict}`) + (c.why ? `: ${c.why}` : '') : (flagged ? T(`research.status.${c.status}`) : '');
                if (why) {
                    const small = document.createElement('small');
                    small.textContent = why;
                    li.append(small);
                }

                out.append(li);
            }
        } catch (err) {
            failed('research-check-said', err);
        } finally {
            el('research-check-run').disabled = false;
        }
    }

    /* ---------- The reports kept ---------- */

    function when(iso) {
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return '';
        return d.toLocaleDateString(document.documentElement.lang === 'bg' ? 'bg-BG' : 'en-GB',
            { day: 'numeric', month: 'short', year: 'numeric' });
    }

    function drawList(data) {
        lastList = data;

        el('research-web').hidden = data.web !== false;

        const ul = el('research-list');
        ul.replaceChildren();

        const reports = data.reports || [];

        for (const r of reports) {
            const li = document.createElement('li');
            li.className = 'writing-item study-item';
            if (r.n_flagged) li.classList.add('is-due');

            const main = document.createElement('div');
            main.className = 'study-item-main';

            const name = document.createElement('strong');
            name.textContent = r.question;

            const stats = document.createElement('small');
            stats.className = 'study-stats';
            stats.textContent = [
                when(r.created_at),
                T(`research.depthName.${r.depth}`),
                T('research.nSources', { n: r.n_sources }),
                T('research.supported', { n: r.n_claims - r.n_flagged, of: r.n_claims }),
            ].filter(Boolean).join(' · ');

            main.append(name, stats);

            const actions = document.createElement('div');
            actions.className = 'study-item-actions';

            const button = (label, fn, cls) => {
                const b = document.createElement('button');
                b.type = 'button';
                b.className = `model-action ${cls || ''}`.trim();
                b.textContent = label;
                b.addEventListener('click', () => fn(b));
                actions.append(b);
            };

            button(T('research.open'), () => open(r.id));
            button(T('study.remove'), (b) => remove(r, b), 'quiet');

            li.append(main, actions);
            ul.append(li);
        }

        el('research-empty').hidden = reports.length > 0;
    }

    async function loadList() {
        try {
            drawList(await get('/api/research'));
        } catch (err) {
            say('research-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function remove(r, button) {
        if (!button.classList.contains('danger')) {
            button.classList.add('danger');
            button.textContent = T('gl.sure');
            setTimeout(() => { button.classList.remove('danger'); button.textContent = T('study.remove'); }, 4000);
            return;
        }

        try {
            await send(`/api/research/${r.id}/remove`);
            say('research-said', T('research.removedReport'));

            if (shown && shown.report.id === r.id) el('research-report-card').hidden = true;

            await loadList();
        } catch (err) {
            say('research-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('research-list')) return;

        el('research-run').addEventListener('click', run);
        el('research-question').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                run();
            }
        });
        el('research-close').addEventListener('click', () => { el('research-report-card').hidden = true; });
        el('research-check-run').addEventListener('click', check);

        for (const item of document.querySelectorAll('.nav-item[data-view="research"]')) {
            item.addEventListener('click', loadList);
        }

        window.addEventListener('pn-language', () => {
            if (lastList) drawList(lastList);
            if (shown && !el('research-report-card').hidden) show(shown, true);
        });

        loadList();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
