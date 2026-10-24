/*
 * What I did and why.
 *
 * Every call the assistant made, put to its owner or refused, read from the
 * record the program keeps (store.Deed) — never from the model's account of
 * itself. One line each, newest first, opening onto the request behind it,
 * who acted, the model and why, what it was given and what came back.
 *
 * Loaded when the page is opened and when a filter changes. Not polled: a
 * list that rewrites itself under somebody reading it is unreadable, and the
 * Refresh button is one press away.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const toolName = (name) => (window.pnToolName ? window.pnToolName(name) : name);

    async function get(path) {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `that did not work (${res.status})`);

        return body.data !== undefined ? body.data : body;
    }

    function took(ms) {
        if (!ms) return '';
        if (ms < 1000) return `${ms} ms`;
        if (ms < 60000) return `${(ms / 1000).toFixed(1)} s`;

        return `${Math.round(ms / 60000)} min`;
    }

    function when(iso) {
        const at = new Date(iso);

        if (Number.isNaN(at.getTime())) return '';

        const lang = window.pnLang ? window.pnLang() : 'en';

        try {
            return at.toLocaleString(lang === 'bg' ? 'bg-BG' : 'en-GB', {
                day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit',
            });
        } catch {
            return at.toISOString().slice(0, 16).replace('T', ' ');
        }
    }

    /*
     * The line a deed is known by. The tool's own summary is written in
     * English for the model; on the Bulgarian page the capability's name is
     * said in Bulgarian, followed by what it was given — an address, a
     * path — which reads the same in either language.
     */
    function headline(d) {
        if (!(window.pnLang && window.pnLang() === 'bg')) return d.summary || toolName(d.tool);

        const first = (d.asked || '').split(' · ')[0];
        const value = first.includes(': ') ? first.slice(first.indexOf(': ') + 2) : '';

        return value ? `${toolName(d.tool)} · ${value}` : toolName(d.tool);
    }

    function field(label, value, mono) {
        const row = document.createElement('div');
        row.className = 'deed-field';

        const k = document.createElement('span');
        k.className = 'deed-k';
        k.textContent = label;

        const v = document.createElement(mono ? 'pre' : 'span');
        v.className = 'deed-v' + (mono ? ' mono' : '');
        // textContent throughout: a result can hold a page that was read.
        v.textContent = value;

        row.append(k, v);

        return row;
    }

    function deed(d) {
        const item = document.createElement('details');
        item.className = 'deed';
        item.dataset.outcome = d.outcome;

        const head = document.createElement('summary');

        const time = document.createElement('span');
        time.className = 'deed-time';
        time.textContent = when(d.created_at);

        const what = document.createElement('span');
        what.className = 'deed-what';
        what.textContent = headline(d);
        what.title = d.summary || toolName(d.tool);

        const outcome = document.createElement('span');
        outcome.className = 'deed-outcome';
        outcome.textContent = T('outcome.' + d.outcome);

        head.append(time, what, outcome);

        if (d.leaves) {
            const left = document.createElement('span');
            left.className = 'deed-leaves';
            left.textContent = '↗ ' + (window.pnLeavesWhat ? window.pnLeavesWhat(d.leaves) : d.leaves);
            left.title = T('deeds.left');
            head.appendChild(left);
        }

        item.appendChild(head);

        const body = document.createElement('div');
        body.className = 'deed-body';

        if (d.request) {
            const asked = d.request === 'you approved it' ? T('deeds.approved') : d.request;

            body.appendChild(field(T('deeds.why'), asked));
        }

        if (d.task_name) body.appendChild(field(T('deeds.task'), d.task_name));

        body.appendChild(field(T('deeds.who'), d.specialist || T('deeds.you')));

        if (d.model) body.appendChild(field(T('deeds.model'), d.model));
        if (d.model_why) body.appendChild(field(T('deeds.modelWhy'), d.model_why));

        const traits = [toolName(d.tool), d.changes_something ? T('deeds.changes') : T('deeds.looks')];

        if (d.level) traits.push(T('deeds.risk') + ': ' + T('risk.' + d.level));
        if (d.millis) traits.push(T('deeds.took', { t: took(d.millis) }));
        if (!d.leaves && (d.outcome === 'done' || d.outcome === 'failed')) traits.push(T('deeds.stayedTrait'));

        const line = document.createElement('p');
        line.className = 'deed-traits';
        line.textContent = traits.join(' · ');
        body.appendChild(line);

        if (d.leaves) {
            body.appendChild(field(T('deeds.left'), window.pnLeavesWhat ? window.pnLeavesWhat(d.leaves) : d.leaves));
        }

        if (d.asked) body.appendChild(field(T('deeds.asked'), d.asked, true));
        if (d.result) body.appendChild(field(T('deeds.result'), d.result, true));

        item.appendChild(body);

        return item;
    }

    function counts(c) {
        const parts = [T('deeds.count', { n: c.all || 0 })];

        if (c.changes) parts.push(T('deeds.countChanges', { n: c.changes }));
        if (c.leaves) parts.push(T('deeds.countLeaves', { n: c.leaves }));
        if (c.waiting) parts.push(T('deeds.countWaiting', { n: c.waiting }));
        if (c.refused) parts.push(T('deeds.countRefused', { n: c.refused }));
        if (c.failed) parts.push(T('deeds.countFailed', { n: c.failed }));

        return parts.join(' · ');
    }

    let asking = 0;

    async function load() {
        const list = el('deeds-list');

        if (!list) return;

        const params = new URLSearchParams({ since: (el('deeds-since') || {}).value || 'week' });

        if (el('deeds-changes') && el('deeds-changes').checked) params.set('changes', '1');
        if (el('deeds-leaves') && el('deeds-leaves').checked) params.set('leaves', '1');

        // Only the newest request is drawn, so a slow answer to an older
        // filter cannot replace a newer one.
        const mine = ++asking;

        let data;

        try {
            data = await get('/api/deeds?' + params.toString());
        } catch (err) {
            if (mine !== asking) return;

            list.textContent = '';
            el('deeds-counts').textContent = T('deeds.error');
            el('deeds-empty').hidden = true;

            return;
        }

        if (mine !== asking) return;

        const deeds = data.deeds || [];

        list.textContent = '';

        for (const d of deeds) list.appendChild(deed(d));

        el('deeds-counts').textContent = deeds.length ? counts(data.counts || {}) : '';

        const empty = el('deeds-empty');

        empty.hidden = deeds.length > 0;
        empty.textContent = T('deeds.empty');
    }

    for (const id of ['deeds-since', 'deeds-changes', 'deeds-leaves']) {
        const control = el(id);

        if (control) control.addEventListener('change', load);
    }

    const refresh = el('deeds-refresh');

    if (refresh) refresh.addEventListener('click', load);

    for (const item of document.querySelectorAll('.nav-item[data-view="deeds"]')) {
        item.addEventListener('click', () => load());
    }

    window.addEventListener('pn-language', () => {
        if (!el('deeds-list')?.closest('[hidden]')) load();
    });

    window.brainShowDeeds = load;
}());
