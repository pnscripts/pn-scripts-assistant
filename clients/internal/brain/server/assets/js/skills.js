/*
 * Skills: every skill the assistant can follow, with where it came from and
 * a switch each, searched as you type; the folders skills are read from;
 * Reload, which reads them all again (server/skillspage.go). A skill is
 * somebody's text, so everything is drawn with textContent; the Bulgarian
 * words of a built-in skill are shown when the screen is in Bulgarian.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => document.documentElement.lang === 'bg';

    let data = null;

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
        const out = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(out.error || `${res.status}`);

        return out.data !== undefined ? out.data : out;
    }

    function say(id, text, bad) {
        const p = el(id);
        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    const when = (s) => (bg() && s.when_bg ? s.when_bg : s.when) || '';
    const how = (s) => (bg() && s.how_bg ? s.how_bg : s.how) || '';
    const builtin = (s) => s.source === 'pack' || s.source === 'playbook';

    function badge(s) {
        const b = document.createElement('span');
        b.className = `plan-chip sk-badge sk-${s.source}`;
        b.textContent = T(`sk.src.${s.source}`);

        if (s.source === 'imported' && s.imported) b.title = s.imported;

        return b;
    }

    function matches(s, q, filter) {
        if (filter === 'builtin' && !builtin(s)) return false;
        if (filter === 'imported' && s.source !== 'imported') return false;
        if (filter === 'taught' && s.source !== 'taught') return false;
        if (filter === 'off' && s.on) return false;
        if (!q) return true;

        const hay = `${s.name} ${s.when || ''} ${s.when_bg || ''}`.toLowerCase();

        return q.split(/\s+/).every((w) => hay.includes(w));
    }

    function draw() {
        if (!data) return;

        const ul = el('sk-list');
        ul.replaceChildren();

        const q = el('sk-search').value.trim().toLowerCase();
        const filter = el('sk-filter').value;
        const list = (data.skills || []).filter((s) => matches(s, q, filter));

        for (const s of list) {
            const li = document.createElement('li');
            li.className = `writing-item study-item sk-item${s.on ? '' : ' is-off'}`;

            const main = document.createElement('div');
            main.className = 'study-item-main';

            const name = document.createElement('strong');
            name.textContent = s.name.replace(/_/g, ' ');

            const desc = document.createElement('small');
            desc.className = 'study-stats';
            desc.textContent = when(s);

            const details = document.createElement('details');
            details.className = 'sk-how';

            const summary = document.createElement('summary');
            summary.textContent = T('sk.instructions');

            const pre = document.createElement('pre');
            details.addEventListener('toggle', () => { if (details.open && !pre.textContent) pre.textContent = how(s); });

            details.append(summary, pre);
            main.append(name, badge(s), desc, details);

            const toggle = document.createElement('label');
            toggle.className = 'sk-toggle';

            const box = document.createElement('input');
            box.type = 'checkbox';
            box.checked = s.on;
            box.setAttribute('aria-label', T('sk.onFor', { name: s.name }));
            box.addEventListener('change', async () => {
                box.disabled = true;

                try {
                    await send(`/api/settings/skills/${encodeURIComponent(s.name)}`, { on: box.checked });
                    s.on = box.checked;
                    li.classList.toggle('is-off', !s.on);
                    say('sk-said', T(s.on ? 'sk.nowOn' : 'sk.nowOff', { name: s.name }));
                    count();
                } catch (err) {
                    box.checked = !box.checked;
                    say('sk-said', T('writing.failed', { why: err.message }), true);
                } finally {
                    box.disabled = false;
                }
            });

            const word = document.createElement('span');
            word.textContent = T('sk.on');

            toggle.append(box, word);
            li.append(main, toggle);
            ul.append(li);
        }

        el('sk-empty').hidden = list.length > 0;
        count();
    }

    function count() {
        const all = data.skills || [];
        el('sk-count').textContent = T('sk.count', { n: all.length, on: all.filter((s) => s.on).length });
    }

    function drawFolders() {
        const ul = el('sk-folders');
        ul.replaceChildren();

        for (const f of data.folders || []) {
            const li = document.createElement('li');
            const strong = document.createElement('strong');
            strong.textContent = T(`sk.kind.${f.kind}`);

            li.append(strong, document.createTextNode(`: ${f.path} — ${f.exists ? T('sk.found', { n: f.skills }) : T('sk.missing')}`));
            ul.append(li);
        }

        el('sk-added').value = (data.added || []).join('\n');
    }

    async function load(quiet) {
        try {
            data = await get('/api/skills/catalogue');
            draw();
            drawFolders();
            if (!quiet) say('sk-said', '');
        } catch (err) {
            say('sk-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function reload() {
        el('sk-reload').disabled = true;

        try {
            const r = await send('/api/skills/reload');
            await load(true);
            say('sk-said', T('sk.reloaded', { n: r.skills }));
        } catch (err) {
            say('sk-said', T('writing.failed', { why: err.message }), true);
        } finally {
            el('sk-reload').disabled = false;
        }
    }

    async function save() {
        const folders = el('sk-added').value.split('\n').map((s) => s.trim()).filter(Boolean);

        try {
            const r = await send('/api/settings/skill-folders', { folders });
            await load(true);
            say('sk-folders-said', T('sk.saved', { n: r.skills }));
        } catch (err) {
            say('sk-folders-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('sk-list')) return;

        el('sk-search').addEventListener('input', draw);
        el('sk-filter').addEventListener('change', draw);
        el('sk-reload').addEventListener('click', reload);
        el('sk-save').addEventListener('click', save);

        for (const item of document.querySelectorAll('.nav-item[data-view="skills"]')) {
            item.addEventListener('click', () => load());
        }

        window.addEventListener('pn-language', () => { if (data) { draw(); drawFolders(); } });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
