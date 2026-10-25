/*
 * Writing and proofreading.
 *
 * Proofread asks the server's rules (the proof package): every correction
 * comes with its reason in the page's language, the sure ones ticked, the
 * others left for a person. Clicking one selects it in the text. Rewrite
 * asks the model for the tone chosen and shows the result as changes, to be
 * kept or discarded — nothing replaces somebody's text without being seen.
 *
 * Positions from the server are byte offsets into UTF-8; the text box counts
 * UTF-16 units, so every position is converted before it is used.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => window.pnLang && window.pnLang() === 'bg';

    let tones = [];
    let report = null;
    let checked = '';
    let draft = null;

    async function send(path, body) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(data.error || `${res.status}`);

        return data.data !== undefined ? data.data : data;
    }

    function say(text, bad) {
        const p = el('writing-said');
        if (!p) return;

        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    function options() {
        const most = parseInt(el('writing-most').value, 10);

        return {
            language: el('writing-language').value,
            tone: el('writing-tone').value,
            max_words: Number.isFinite(most) && most > 0 ? most : 0,
        };
    }

    // The UTF-16 index of a UTF-8 byte offset in text.
    function indexOf(text, bytes) {
        let seen = 0;
        let i = 0;

        for (const ch of text) {
            if (seen >= bytes) break;

            const code = ch.codePointAt(0);
            seen += code < 0x80 ? 1 : code < 0x800 ? 2 : code < 0x10000 ? 3 : 4;
            i += ch.length;
        }

        return i;
    }

    function stats(s) {
        el('writing-stats').textContent = s && s.words ? T('writing.stats', {
            words: s.words, sentences: s.sentences, longest: s.longest_sentence, minutes: s.reading_minutes,
        }) : '';
    }

    function shown(text) {
        const flat = String(text || '').replace(/\s+/g, ' ').trim();

        return flat.length > 70 ? flat.slice(0, 69) + '…' : flat;
    }

    /*
     * What a finding is shown as. A missing comma or an extra space is
     * invisible on its own, and "е" taken out of "е е" says nothing without
     * the word beside it — so those are shown with the words round them,
     * before and after.
     */
    function view(f) {
        const deletes = !f.suggest && (f.rule === 'repeated_word' || f.rule === 'space_before_punctuation');
        const blank = !String(f.found || '').trim() || f.rule === 'double_space';

        if (!(blank || deletes) || !checked || !f.length) {
            return { was: shown(f.found), now: f.suggest ? shown(f.suggest) : (deletes ? T('writing.remove') : '') };
        }

        const start = indexOf(checked, f.at);
        const end = indexOf(checked, f.at + f.length);

        let from = start;
        while (from > 0 && /\s/.test(checked[from - 1])) from--;
        while (from > 0 && !/\s/.test(checked[from - 1])) from--;

        let to = end;
        while (to < checked.length && /[ \t]/.test(checked[to])) to++;
        while (to < checked.length && !/\s/.test(checked[to])) to++;

        // Spaces are what a spacing slip is about, so they are made visible
        // rather than collapsed.
        if (f.rule === 'double_space' || f.rule === 'space_before_punctuation') {
            const mark = (t) => t.replace(/ /g, '␣');

            return {
                was: mark(checked.slice(from, to)),
                now: mark(checked.slice(from, start) + (f.suggest || '') + checked.slice(end, to)),
            };
        }

        return {
            was: shown(checked.slice(from, to)),
            now: shown(checked.slice(from, start) + (f.suggest || '') + checked.slice(end, to)),
        };
    }

    function finding(f, i, tickable) {
        const li = document.createElement('li');
        li.className = 'writing-item' + (f.sure ? ' is-sure' : '');

        if (tickable) {
            const box = document.createElement('input');
            box.type = 'checkbox';
            box.checked = !!f.sure;
            box.dataset.index = String(i);
            box.disabled = !f.suggest && f.rule !== 'repeated_word' && f.rule !== 'space_before_punctuation';
            box.setAttribute('aria-label', T('writing.use'));
            li.append(box);
        }

        const body = document.createElement('button');
        body.type = 'button';
        body.className = 'writing-pick';

        const what = document.createElement('span');
        what.className = 'writing-what';

        const shownAs = view(f);

        const found = document.createElement('span');
        found.className = tickable ? 'diff-del' : 'writing-found-text';
        found.textContent = shownAs.was || '␣';
        what.append(found);

        if (shownAs.now) {
            const arrow = document.createElement('span');
            arrow.className = 'writing-arrow';
            arrow.textContent = '→';

            const put = document.createElement('span');
            put.className = 'diff-ins';
            put.textContent = shownAs.now;

            what.append(arrow, put);
        }

        const why = document.createElement('span');
        why.className = 'writing-why';
        why.textContent = (bg() ? f.why_bg : f.why) + (f.sure || !tickable ? '' : ` (${T('writing.checkIt')})`);

        body.append(what, why);

        // Shows where it is in the text, if the text is still the one checked.
        body.addEventListener('click', () => {
            const box = el('writing-text');
            if (box.value !== checked || !f.length) return;

            const start = indexOf(checked, f.at);
            const end = indexOf(checked, f.at + f.length);

            box.focus();
            box.setSelectionRange(start, end);
        });

        li.append(body);

        return li;
    }

    function draw() {
        const slips = el('writing-slips');
        const notes = el('writing-notes');

        slips.replaceChildren();
        notes.replaceChildren();

        if (!report) {
            el('writing-found').hidden = true;
            return;
        }

        stats(report.stats);

        report.findings.forEach((f, i) => slips.append(finding(f, i, true)));
        report.tone.forEach((f, i) => notes.append(finding(f, i, false)));

        el('writing-found').hidden = report.findings.length + report.tone.length === 0;
        el('writing-apply').hidden = report.findings.length === 0;
        el('writing-tone-title').hidden = report.tone.length === 0;

        if (report.findings.length + report.tone.length === 0) say(T('writing.none'));
    }

    async function check() {
        const text = el('writing-text').value;

        if (!text.trim()) {
            say(T('writing.empty'), true);
            return;
        }

        say('');

        try {
            report = await send('/api/writing/check', { text, ...options() });
            checked = text;
            draw();
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        }
    }

    async function apply() {
        const text = el('writing-text').value;

        if (text !== checked) {
            await check();
            return;
        }

        const chosen = [...el('writing-slips').querySelectorAll('input[type=checkbox]:checked')]
            .map((box) => parseInt(box.dataset.index, 10));

        try {
            const done = await send('/api/writing/apply', {
                text, language: el('writing-language').value, chosen,
            });

            el('writing-text').value = done.text;
            await check();
            say(T('writing.applied', { n: chosen.length }));
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        }
    }

    function diff(into, parts) {
        into.replaceChildren();

        for (const part of parts || []) {
            const span = document.createElement(part.kind === 'same' ? 'span' : part.kind === 'added' ? 'ins' : 'del');
            if (part.kind === 'added') span.className = 'diff-ins';
            if (part.kind === 'removed') span.className = 'diff-del';
            span.textContent = part.text;
            into.append(span);
        }
    }

    async function rewrite() {
        const text = el('writing-text').value;
        const opts = options();

        if (!text.trim()) {
            say(T('writing.empty'), true);
            return;
        }

        if (!opts.tone) {
            say(T('writing.needTone'), true);
            el('writing-tone').focus();
            return;
        }

        const button = el('writing-rewrite');
        button.disabled = true;
        say(T('writing.working'));

        try {
            draft = await send('/api/writing/rewrite', { text, ...opts });

            el('writing-new-counts').textContent = T('writing.rewritten', {
                model: draft.model || '—', removed: draft.diff.words_removed, added: draft.diff.words_added,
            });
            diff(el('writing-new-diff'), draft.diff.parts);
            el('writing-new').hidden = false;
            say('');
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        } finally {
            button.disabled = false;
        }
    }

    function keep() {
        if (!draft) return;

        el('writing-text').value = draft.text;
        report = draft.report;
        checked = draft.text;
        draft = null;
        el('writing-new').hidden = true;
        draw();
        say(T('writing.kept'));
    }

    function discard() {
        draft = null;
        el('writing-new').hidden = true;
        say('');
    }

    async function compare() {
        const before = el('writing-before').value;
        const after = el('writing-after').value;
        const view = el('writing-compare-diff');

        if (!before.trim() && !after.trim()) {
            el('writing-compare-counts').textContent = T('writing.empty');
            view.hidden = true;
            return;
        }

        try {
            const d = await send('/api/writing/compare', { before, after });
            const changed = d.words_removed + d.words_added > 0;

            el('writing-compare-counts').textContent = changed
                ? T('writing.diffCounts', { removed: d.words_removed, added: d.words_added, same: d.words_same })
                : T('writing.same');

            diff(view, d.parts);
            view.hidden = !changed;
        } catch (err) {
            el('writing-compare-counts').textContent = T('writing.failed', { why: err.message });
        }
    }

    async function copy() {
        try {
            await navigator.clipboard.writeText(el('writing-text').value);
            say(T('writing.copied'));
        } catch {
            el('writing-text').select();
        }
    }

    function fillTones() {
        const select = el('writing-tone');
        const chosen = select.value;

        select.querySelectorAll('option[data-tone]').forEach((o) => o.remove());

        for (const t of tones) {
            const o = document.createElement('option');
            o.value = t.name;
            o.dataset.tone = '1';
            o.textContent = bg() ? t.label_bg : t.label;
            select.append(o);
        }

        select.value = chosen;
    }

    async function load() {
        if (tones.length) return;

        try {
            const res = await fetch('/api/writing/tones', { headers: { Accept: 'application/json' } });
            const body = await res.json();

            tones = (body.data !== undefined ? body.data : body).tones || [];
            fillTones();
        } catch { /* the page still proofreads without tones */ }
    }

    function wire() {
        if (!el('writing-text')) return;

        el('writing-check').addEventListener('click', check);
        el('writing-apply').addEventListener('click', apply);
        el('writing-rewrite').addEventListener('click', rewrite);
        el('writing-keep').addEventListener('click', keep);
        el('writing-discard').addEventListener('click', discard);
        el('writing-compare').addEventListener('click', compare);
        el('writing-copy').addEventListener('click', copy);

        // Ctrl+Enter proofreads, the way Enter sends in the chat.
        el('writing-text').addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                check();
            }
        });

        for (const item of document.querySelectorAll('.nav-item[data-view="writing"]')) {
            item.addEventListener('click', load);
        }

        window.addEventListener('pn-language', () => {
            fillTones();
            if (report) draw();
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
