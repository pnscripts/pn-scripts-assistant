/*
 * Translation and the glossary.
 *
 * A text is translated by the model through the brain; a file is translated
 * into a new file beside it, with its layout kept. Both come back with the
 * glossary terms they used and anything the check found missing — a term not
 * translated as agreed, a number or a link that did not survive.
 *
 * The glossary is the owner's list of agreed translations, kept in the brain
 * folder; every translation is given it and checked against it.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => window.pnLang && window.pnLang() === 'bg';

    let languages = [];
    let terms = [];
    let loaded = false;

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

    function say(id, text, bad) {
        const p = el(id);
        if (!p) return;

        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    function name(code) {
        const l = languages.find((x) => x.code === code);
        if (!l) return code || T('gl.anyShort');

        return bg() ? l.name_bg : l.name;
    }

    // Fills a language list, keeping what was chosen.
    function fill(select, keepFirst, fallback) {
        const chosen = select.value || fallback;

        [...select.options].forEach((o, i) => { if (!(keepFirst && i === 0)) o.remove(); });

        for (const l of languages) {
            const o = document.createElement('option');
            o.value = l.code;
            o.textContent = bg() ? l.name_bg : l.name;
            select.append(o);
        }

        select.value = chosen;
        if (select.value !== chosen && fallback !== undefined) select.value = fallback;
    }

    function notes(list, res) {
        list.replaceChildren();

        const add = (text, warn) => {
            const li = document.createElement('li');
            li.className = 'writing-item' + (warn ? '' : ' is-sure');

            const span = document.createElement('span');
            span.className = 'writing-why';
            span.textContent = text;

            li.append(span);
            list.append(li);
        };

        if (res.terms_used && res.terms_used.length) {
            add(T('tr.used', { terms: res.terms_used.map((t) => `${t.source} → ${t.target}`).join('; ') }), false);
        }

        for (const m of res.missed || []) {
            add(m.kind === 'term'
                ? T('tr.missTerm', { source: m.source, want: m.want })
                : T('tr.missKept', { source: m.source }), true);
        }
    }

    async function translateText() {
        const text = el('tr-text').value;

        if (!text.trim()) {
            say('tr-said', T('writing.empty'), true);
            return;
        }

        const button = el('tr-go');
        button.disabled = true;
        say('tr-said', T('tr.working'));

        try {
            const res = await send('/api/translate', { text, from: el('tr-from').value, to: el('tr-to').value });

            el('tr-out').value = res.text;
            notes(el('tr-notes'), res);
            say('tr-said', res.missed.length ? T('tr.checkThese') : T('tr.done'), res.missed.length > 0);
        } catch (err) {
            say('tr-said', T('writing.failed', { why: err.message }), true);
        } finally {
            button.disabled = false;
        }
    }

    async function translateFile() {
        const path = el('tr-path').value.trim();

        if (!path) {
            say('tr-file-said', T('tr.needPath'), true);
            el('tr-path').focus();
            return;
        }

        const button = el('tr-file');
        button.disabled = true;
        say('tr-file-said', T('tr.fileWorking'));
        el('tr-file-notes').replaceChildren();

        try {
            const res = await send('/api/translate/file', { path, from: el('tr-from').value, to: el('tr-to').value });

            say('tr-file-said', T('tr.fileDone', { n: res.segments, path: res.text }));
            notes(el('tr-file-notes'), res);
        } catch (err) {
            say('tr-file-said', T('writing.failed', { why: err.message }), true);
        } finally {
            button.disabled = false;
        }
    }

    function drawTerms() {
        const rows = el('gl-rows');
        const only = el('gl-filter').value;
        const shown = terms.filter((t) => !only || t.to === only);

        rows.replaceChildren();

        for (const t of shown) {
            const tr = document.createElement('tr');

            const cells = [t.source, t.target, `${name(t.from)} → ${name(t.to)}`, t.note || ''];

            cells.forEach((text, i) => {
                const td = document.createElement('td');
                td.textContent = text;
                // An empty note has no label either, so a phone does not show
                // a heading over nothing.
                td.dataset.label = text ? T(['gl.source', 'gl.target', 'gl.langs', 'gl.noteShort'][i]) : '';
                tr.append(td);
            });

            const td = document.createElement('td');
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'model-action';
            remove.textContent = T('gl.remove');
            remove.setAttribute('aria-label', T('gl.removeOne', { term: t.source }));

            // A second press confirms, the way other removals here do.
            remove.addEventListener('click', async () => {
                if (!remove.classList.contains('danger')) {
                    remove.classList.add('danger');
                    remove.textContent = T('gl.sure');
                    setTimeout(() => {
                        remove.classList.remove('danger');
                        remove.textContent = T('gl.remove');
                    }, 4000);
                    return;
                }

                try {
                    await send('/api/glossary/remove', { source: t.source, from: t.from || '', to: t.to });
                    say('gl-said', T('gl.removed', { term: t.source }));
                    await loadTerms();
                } catch (err) {
                    say('gl-said', T('writing.failed', { why: err.message }), true);
                }
            });

            td.append(remove);
            tr.append(td);
            rows.append(tr);
        }

        el('gl-table').hidden = shown.length === 0;
        el('gl-empty').hidden = shown.length !== 0;
        el('gl-count').textContent = terms.length ? T('gl.count', { n: shown.length, all: terms.length }) : '';
    }

    async function loadTerms() {
        const res = await fetch('/api/glossary', { headers: { Accept: 'application/json' } });
        const body = await res.json();
        const data = body.data !== undefined ? body.data : body;

        terms = data.terms || [];

        if (!loaded) {
            languages = data.languages || [];
            el('tr-formats').textContent = T('tr.formats', { formats: (data.formats || []).join(', ') });
            drawLanguages();
            loaded = true;
        }

        drawTerms();
    }

    function drawLanguages() {
        const home = bg() ? 'bg' : 'en';
        const other = home === 'bg' ? 'en' : 'bg';

        fill(el('tr-from'), true, 'auto');
        fill(el('tr-to'), false, other);
        fill(el('gl-from'), true, '');
        fill(el('gl-to'), false, other);
        fill(el('gl-filter'), true, '');
    }

    async function addTerm() {
        const body = {
            source: el('gl-source').value, target: el('gl-target').value,
            from: el('gl-from').value, to: el('gl-to').value, note: el('gl-note').value,
        };

        try {
            const res = await send('/api/glossary', body);

            say('gl-said', T(res.replaced ? 'gl.changed' : 'gl.added', { term: res.term.source, target: res.term.target }));
            el('gl-source').value = '';
            el('gl-target').value = '';
            el('gl-note').value = '';
            el('gl-source').focus();
            await loadTerms();
        } catch (err) {
            say('gl-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function importTerms() {
        try {
            const res = await send('/api/glossary/import', {
                csv: el('gl-csv').value, from: el('gl-from').value, to: el('gl-to').value,
            });

            say('gl-said', T('gl.imported', { n: res.added })
                + (res.problems.length ? ' ' + T('gl.problems', { list: res.problems.join('; ') }) : ''), res.problems.length > 0);

            if (res.added) el('gl-csv').value = '';
            await loadTerms();
        } catch (err) {
            say('gl-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('tr-text')) return;

        el('tr-go').addEventListener('click', translateText);
        el('tr-file').addEventListener('click', translateFile);
        el('gl-add').addEventListener('click', addTerm);
        el('gl-import').addEventListener('click', importTerms);
        el('gl-filter').addEventListener('change', drawTerms);
        el('tr-copy').addEventListener('click', async () => {
            try {
                await navigator.clipboard.writeText(el('tr-out').value);
                say('tr-said', T('writing.copied'));
            } catch {
                el('tr-out').select();
            }
        });

        for (const id of ['gl-source', 'gl-target', 'gl-note']) {
            el(id).addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    addTerm();
                }
            });
        }

        el('tr-text').addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                translateText();
            }
        });

        for (const item of document.querySelectorAll('.nav-item[data-view="translation"]')) {
            item.addEventListener('click', () => loadTerms().catch(() => {}));
        }

        window.addEventListener('pn-language', () => {
            if (!loaded) return;

            drawLanguages();
            drawTerms();
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
