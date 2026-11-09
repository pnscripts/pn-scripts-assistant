/*
 * The last look before sending, on the Writing page (server/rules.go,
 * package polish): the text as it should go, what was put right, and what is
 * left for the writer — a placeholder, a wrong weekday. "Use this text"
 * puts it back in the box. Drawn with textContent: the text is the user's.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    let last = null;

    // Quoted as each language quotes.
    const q = (s) => (document.documentElement.lang === 'bg' ? `„${s}“` : `“${s}”`);

    function say(text, bad) {
        const p = el('writing-said');
        if (!p) return;
        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    function draw() {
        if (!last) return;

        const list = el('polish-findings');
        list.replaceChildren();

        for (const f of last.findings || []) {
            const li = document.createElement('li');
            li.className = f.fixed ? 'polish-fixed' : 'polish-open';

            const what = document.createElement('strong');
            what.textContent = T(`polish.f.${f.key}`);

            li.append(what);

            if (f.found) li.append(document.createTextNode(`: ${q(f.found)}`));
            if (f.hint && f.key !== 'slip') li.append(document.createTextNode(` (${f.hint}?)`));
            if (f.hint && f.key === 'slip') li.append(document.createTextNode(` → ${q(f.hint)}`));

            const state = document.createElement('small');
            state.className = 'study-stats';
            state.textContent = ` · ${T(f.fixed ? 'polish.fixed' : 'polish.open')}`;
            li.append(state);

            list.append(li);
        }

        let state = 'polish.nothing';
        if (last.blocking) state = 'polish.blocking';
        else if ((last.findings || []).length) state = 'polish.changed';

        el('polish-state').textContent = T(state);
        el('polish-state').classList.toggle('is-bad', !!last.blocking);
        el('polish-text').textContent = last.text;
        el('polish-use').hidden = !last.changed;
        el('writing-polished').hidden = false;
    }

    async function polish() {
        const text = el('writing-text').value;

        if (!text.trim()) {
            say(T('writing.empty'), true);

            return;
        }

        const lang = el('writing-language').value;

        try {
            const res = await fetch('/api/writing/polish', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify({ text, lang: lang === 'auto' ? '' : lang }),
            });
            const out = await res.json().catch(() => ({}));

            if (!res.ok) throw new Error(out.error || `${res.status}`);

            last = out;
            say('');
            draw();
            el('writing-polished').scrollIntoView({ block: 'nearest' });
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        }
    }

    function use() {
        if (!last) return;

        el('writing-text').value = last.text;
        el('writing-polished').hidden = true;
        say(T('polish.used'));
    }

    function wire() {
        if (!el('writing-polish')) return;

        el('writing-polish').addEventListener('click', polish);
        el('polish-use').addEventListener('click', use);
        window.addEventListener('pn-language', draw);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
