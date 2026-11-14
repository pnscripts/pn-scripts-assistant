/*
 * The Programs page: what is installed on this computer, what came and went,
 * and, for each program, what the assistant found and what it does with it.
 *
 * Read only. The server keeps the inventory current by itself (package
 * programs); this draws GET /api/programs. The words are worked out by small
 * functions on window.pnPrograms, so a test can run them in node on the real
 * server's answer in both languages; the drawing below only places them.
 */
(function () {
    'use strict';

    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => (window.pnLang ? window.pnLang() === 'bg' : false);

    // How far back "What changed" looks, and how many rows a list shows
    // before it asks for a narrower search (said on the page, never cut
    // silently).
    const RECENT_DAYS = 14;
    const MOST_CHANGES = 20;
    const MOST_ROWS = 200;

    // The server's reasons for polling a folder (programs.Reasons), and the
    // key that says each one here.
    const WHY = {
        missing: 'prg.why.missing',
        limit: 'prg.why.limit',
        'no-events': 'prg.why.noEvents',
        'no-path': 'prg.why.noPath',
    };

    // Each source, and its name for a person.
    const KIND = {
        app: 'prg.kind.app',
        deb: 'prg.kind.deb',
        snap: 'prg.kind.snap',
        flatpak: 'prg.kind.flatpak',
        win: 'prg.kind.win',
        msix: 'prg.kind.msix',
    };

    // Each source as the summary counts it.
    const PART = {
        app: 'prg.part.app',
        deb: 'prg.part.deb',
        snap: 'prg.part.snap',
        flatpak: 'prg.part.flatpak',
        win: 'prg.part.win',
        msix: 'prg.part.msix',
    };

    // The sources an answer from an older server names none of: Linux's.
    const LINUX = ['app', 'deb', 'snap', 'flatpak'];

    // The sources worth showing: the server's (this computer's, then any
    // a remembered program came from).
    function kinds(body) {
        const k = body && Array.isArray(body.kinds) ? body.kinds.filter((x) => KIND[x]) : [];
        return k.length ? k : LINUX;
    }

    // The choices of the "Which programs" list: each source shown, then
    // the removed ones.
    function choices(body) {
        return kinds(body).concat(['removed']);
    }

    function when(at) {
        const d = new Date(at);
        if (!at || Number.isNaN(d.getTime()) || d.getUTCFullYear() < 2000) return '';

        const two = (n) => String(n).padStart(2, '0');
        const time = two(d.getHours()) + ':' + two(d.getMinutes());

        if (bg()) return two(d.getDate()) + '.' + two(d.getMonth() + 1) + '.' + d.getFullYear() + ', ' + time;

        return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) + ', ' + time;
    }

    // A count as the page's language writes it (2,855 or 2855; 12 345 or
    // 12,345).
    function count(n) {
        try {
            return new Intl.NumberFormat(bg() ? 'bg-BG' : 'en-GB').format(n);
        } catch (err) {
            return String(n);
        }
    }

    function kindWord(kind) {
        return KIND[kind] ? T(KIND[kind]) : kind;
    }

    // A reason by its code; the server's English for one this page does
    // not know yet.
    function why(polled) {
        return WHY[polled.reason] ? T(WHY[polled.reason]) : polled.why;
    }

    // One sentence for the whole inventory and how it is kept current.
    function summary(body) {
        const all = (body && body.programs) || [];
        const here = all.filter((p) => p.present);
        const of = (kind) => count(here.filter((p) => p.kind === kind).length);

        if (!all.length) {
            return (body && body.first_looks && Object.keys(body.first_looks).length)
                ? T('prg.noneFound') : T('prg.notYet');
        }

        const parts = kinds(body).map((kind) => T(PART[kind], { n: of(kind) }));

        return T('prg.summary', { n: count(here.length), parts: parts.join(', ') });
    }

    // How each source is kept current: watched folders; folders that are
    // not there yet, counted (normal: most computers have no Flatpak); and
    // each folder polled for another reason, by name.
    function keeping(body) {
        const sources = (body && body.sources) || [];

        if (!sources.length) return [T('prg.keepNotStarted')];

        const watched = sources.reduce((n, s) => n + (s.watched || 0), 0);
        const lines = [T('prg.keepWatched', { n: count(watched) })];
        let missing = 0;

        for (const s of sources) {
            for (const p of s.polled || []) {
                if (p.reason === 'missing') {
                    missing++;
                    continue;
                }

                lines.push(p.dir
                    ? T('prg.keepPolled', { kind: kindWord(s.kind), dir: p.dir, why: why(p) })
                    : T('prg.keepPolledSource', { kind: kindWord(s.kind), why: why(p) }));
            }
        }

        if (missing) lines.push(T('prg.keepMissing', { n: count(missing) }));

        return lines;
    }

    // What happened to a program, in a few words.
    function happened(p) {
        switch (p.last_change) {
        case 'added':
            return T('prg.did.added', { when: when(p.changed_at) });
        case 'removed':
            return T('prg.did.removed', { when: when(p.removed_at || p.changed_at) });
        case 'returned':
            return T('prg.did.returned', { when: when(p.changed_at) });
        case 'updated':
            return p.previous_version
                ? T('prg.did.updated', { from: p.previous_version, to: p.version || '?', when: when(p.changed_at) })
                : T('prg.did.updatedPlain', { when: when(p.changed_at) });
        default:
            return '';
        }
    }

    // What changed in the last RECENT_DAYS days, newest first.
    function changes(body, now) {
        const since = (now || Date.now()) - RECENT_DAYS * 86400000;
        const all = ((body && body.programs) || []).filter((p) => p.last_change &&
            new Date(p.changed_at).getTime() >= since);

        all.sort((a, b) => new Date(b.changed_at) - new Date(a.changed_at));

        return {
            total: all.length,
            rows: all.slice(0, MOST_CHANGES).map((p) => ({ program: p, name: p.name || p.id, said: kindWord(p.kind) + ' · ' + happened(p) })),
            more: Math.max(0, all.length - MOST_CHANGES),
        };
    }

    // The list for one choice (a source, or 'removed') and a search, by
    // name.
    function list(body, choice, query) {
        const q = (query || '').trim().toLowerCase();
        const all = ((body && body.programs) || []).filter((p) => (choice === 'removed'
            ? !p.present
            : p.present && p.kind === choice));
        const found = q ? all.filter((p) => [p.name, p.id, p.exec].some((s) => s && s.toLowerCase().includes(q))) : all;

        found.sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id));

        const rows = found.slice(0, MOST_ROWS).map((p) => ({
            program: p,
            name: p.name || p.id,
            said: choice === 'removed'
                ? kindWord(p.kind) + ' · ' + T('prg.did.removed', { when: when(p.removed_at || p.changed_at) })
                // A menu entry has no version; what it starts tells two
                // entries with one name apart.
                : (p.kind === 'app' ? (p.exec || '') : (p.version || '')),
        }));

        let note = '';
        if (!found.length) note = q ? T('prg.noMatch', { q: query.trim() }) : T('prg.emptyChoice');
        else if (found.length > MOST_ROWS) note = T('prg.showing', { shown: count(MOST_ROWS), n: count(found.length) });

        return { rows, total: found.length, note };
    }

    // The first-contact card: what was found about one program, as
    // [label, value] rows; nothing empty is shown.
    function found(p, body) {
        const rows = [[T('prg.f.kind'), kindWord(p.kind)], [T('prg.f.id'), p.id]];

        if (p.version) rows.push([T('prg.f.version'), p.version]);
        if (p.path) rows.push([T('prg.f.path'), p.path]);
        if (p.exec) rows.push([T('prg.f.exec'), p.exec]);

        const looked = body && body.first_looks ? body.first_looks[p.kind] : '';

        rows.push([T('prg.f.since'), p.initial
            ? (looked ? T('prg.f.before', { when: when(looked) }) : T('prg.f.beforePlain'))
            : when(p.first_seen)]);

        const last = happened(p);
        if (last && !(p.last_change === 'added' && p.changed_at === p.first_seen)) rows.push([T('prg.f.last'), last]);

        return rows;
    }

    // What the assistant does with it, today.
    function canDo(p) {
        return p.present ? T('prg.canDo') : T('prg.canDoGone', { when: when(p.removed_at || p.changed_at) });
    }

    // The kinds of use a card decides, in the server's order (an older
    // server sends none: these three).
    const VERBS = ['start', 'control-input', 'change'];

    // Each kind of use, and its name for a person.
    const VERB = {
        start: 'prg.verb.start',
        'control-input': 'prg.verb.control-input',
        change: 'prg.verb.change',
    };

    // The answers, in the order the buttons stand, and their words.
    const ANSWERS = ['ask', 'allow', 'never'];
    const ANSWER = { ask: 'prg.ans.ask', allow: 'prg.ans.allow', never: 'prg.ans.never' };

    /*
     * What the assistant may do with a program without asking: one row per
     * kind of use, with the answer chosen and what it means now. An
     * "always" given to an older version sleeps (the server says asleep);
     * on "ask every time" no "always" is acted on (honoured false); a
     * removed program's choices are kept for when it comes back, and said
     * as kept, not as done.
     */
    function may(p, body) {
        const verbs = body && Array.isArray(body.verbs) && body.verbs.length ? body.verbs : VERBS;
        const decided = (body && body.permits && body.permits[p.kind + ':' + p.id]) || [];
        const honoured = !(body && body.honoured === false);

        const rows = verbs.map((verb) => {
            const d = decided.find((x) => x.verb === verb);
            const answer = d ? d.answer : 'ask';
            let state;

            if (!p.present && answer === 'never') state = T('prg.state.keptNever');
            else if (!p.present && answer === 'allow') state = T('prg.state.keptAllow');
            else if (answer === 'never') state = T('prg.state.never');
            else if (answer === 'allow' && d.asleep) state = T('prg.state.asleep', { from: d.version || '?', to: p.version || '?' });
            else if (answer === 'allow' && !honoured) state = T('prg.state.notHonoured');
            else if (answer === 'allow') state = T('prg.state.allow', { when: when(d.at) });
            else if (verb === 'control-input') state = T('prg.state.askInput');
            else state = T('prg.state.general');

            return { verb, label: VERB[verb] ? T(VERB[verb]) : verb, answer, state };
        });

        return { rows, note: p.present ? '' : T('prg.mayGone') };
    }

    window.pnPrograms = { summary, choices, keeping, changes, list, found, canDo, may, why, reasons: Object.keys(WHY), answers: ANSWERS };

    // ---- drawing ----

    const el = (id) => document.getElementById(id);
    let last = null;
    let chosen = null;

    function say(id, text) {
        const node = el(id);
        if (node) node.textContent = text || '';
    }

    function row(item) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'program-row';

        const name = document.createElement('span');
        name.className = 'program-name';
        name.textContent = item.name;
        b.appendChild(name);

        const what = document.createElement('span');
        what.className = 'program-said';
        what.textContent = item.said;
        b.appendChild(what);

        b.addEventListener('click', () => open(item.program));

        return b;
    }

    function fill(id, items) {
        const box = el(id);
        if (!box) return;

        box.textContent = '';
        for (const item of items) box.appendChild(row(item));
    }

    function open(p) {
        chosen = p;
        saying = '';
        drawCard();

        const card = el('programs-card');
        if (card && card.scrollIntoView) card.scrollIntoView({ block: 'nearest' });
    }

    function drawCard() {
        const card = el('programs-card');
        if (!card) return;

        card.hidden = !chosen;
        if (!chosen) return;

        // The newest copy of the chosen program, after a reload.
        const now = ((last && last.programs) || []).find((p) => p.kind === chosen.kind && p.id === chosen.id) || chosen;

        say('programs-card-name', now.name || now.id);

        const box = el('programs-card-found');
        if (box) {
            box.textContent = '';

            for (const [label, value] of found(now, last)) {
                const dt = document.createElement('dt');
                dt.textContent = label;

                const dd = document.createElement('dd');
                dd.textContent = value;

                box.appendChild(dt);
                box.appendChild(dd);
            }
        }

        say('programs-card-can', canDo(now));
        drawMay(now);
    }

    // The card's choices: three buttons per kind of use, the chosen one
    // pressed; a press is saved at once and the card drawn again from the
    // server's answer.
    let saying = '';

    function drawMay(p) {
        const box = el('programs-card-may');
        if (!box) return;

        const m = may(p, last);
        box.textContent = '';

        for (const r of m.rows) {
            const item = document.createElement('div');
            item.className = 'permit program-permit';
            item.dataset.decision = r.answer === 'never' ? 'refuse' : r.answer;

            const text = document.createElement('span');
            text.className = 'permit-text';

            const name = document.createElement('span');
            name.className = 'permit-name as-typed';
            name.textContent = r.label;
            text.appendChild(name);

            const what = document.createElement('span');
            what.className = 'program-permit-state';
            what.textContent = r.state;
            text.appendChild(what);

            item.appendChild(text);

            const group = document.createElement('div');
            group.className = 'program-permit-choices';
            group.setAttribute('role', 'group');
            group.setAttribute('aria-label', r.label);

            for (const answer of ANSWERS) {
                const b = document.createElement('button');
                b.type = 'button';
                b.className = 'model-action';
                b.dataset.answer = answer;
                b.textContent = T(ANSWER[answer]);
                b.setAttribute('aria-pressed', String(answer === r.answer));
                b.addEventListener('click', () => decide(p, r.verb, answer));
                group.appendChild(b);
            }

            item.appendChild(group);
            box.appendChild(item);
        }

        say('programs-card-may-note', [m.note, saying].filter(Boolean).join(' '));
    }

    async function decide(p, verb, answer) {
        saying = '';

        try {
            const res = await fetch('/api/programs/permit', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify({ kind: p.kind, id: p.id, verb, answer }),
            });

            if (!res.ok) {
                // The server's reason is English; the page says it in
                // its own words, with the code for a phone (403).
                saying = T(res.status === 403 ? 'prg.mayDesk' : 'prg.mayFailed');
                drawCard();
                return;
            }
        } catch (err) {
            saying = T('prg.mayFailed');
            drawCard();
            return;
        }

        await load();
    }

    function draw() {
        if (!last) return;

        say('programs-summary', summary(last));

        const keep = el('programs-keeping');
        if (keep) {
            keep.textContent = '';

            for (const line of keeping(last)) {
                const li = document.createElement('li');
                li.textContent = line;
                keep.appendChild(li);
            }
        }

        const c = changes(last);
        fill('programs-changes', c.rows);
        say('programs-changes-note', c.total ? (c.more ? T('prg.moreChanges', { n: count(c.more) }) : '') : T('prg.noChanges', { days: RECENT_DAYS }));

        // Only the sources this computer has (and any a remembered
        // program came from) are offered.
        const select = el('programs-choice');
        const offered = choices(last);

        if (select) {
            for (const option of select.options) {
                const off = !offered.includes(option.value);
                option.hidden = off;
                option.disabled = off;
            }

            if (!offered.includes(select.value)) select.value = offered[0];
        }

        const l = list(last, select?.value || 'app', el('programs-search')?.value || '');
        fill('programs-list', l.rows);
        say('programs-list-note', l.note);

        drawCard();
    }

    async function load() {
        try {
            const res = await fetch('/api/programs');
            if (!res.ok) {
                say('programs-summary', T('prg.failed'));
                return;
            }

            const body = await res.json();
            last = body && body.data ? body.data : body;
            draw();
        } catch (err) {
            say('programs-summary', T('prg.failed'));
        }
    }

    function wire() {
        document.querySelectorAll('[data-view="programs"]').forEach((b) => {
            if (b.tagName === 'BUTTON') b.addEventListener('click', load);
        });

        el('programs-choice')?.addEventListener('change', draw);
        el('programs-search')?.addEventListener('input', draw);
        el('programs-card-close')?.addEventListener('click', () => {
            chosen = null;
            saying = '';
            drawCard();
        });

        window.addEventListener('pn-language', draw);

        // A program installed while the page is open shows up within a
        // minute; nothing is asked of the server while the page is closed.
        setInterval(() => {
            const view = document.querySelector('section.view[data-view="programs"]');
            if (view && !view.hidden) load();
        }, 30000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
