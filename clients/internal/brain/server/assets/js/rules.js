/*
 * Rules: what its user has told the assistant always or never to do
 * (server/rules.go). Proposals wait at the top to be kept — with their words
 * changed first if wanted — or declined; kept rules have a switch, an editor
 * and a delete that asks once more on the page itself. Also the card under
 * an answer when a rule was noticed in the message (window.pnRuleCard).
 *
 * A rule is its user's words, so everything is drawn with textContent.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => document.documentElement.lang === 'bg';

    let data = null;
    let editing = 0;

    async function call(method, path, body) {
        const res = await fetch(path, {
            method,
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        const out = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(out.error || `${res.status}`);

        return out;
    }

    function say(id, text, bad) {
        const p = el(id);
        if (!p) return;
        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    function day(d) {
        return T(`rules.day.${d}`);
    }

    // "8 November 2026" / "8 ноември 2026": the Bulgarian strings add their
    // own "г.", so a sentence never ends in two full stops.
    function when(iso) {
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return '';

        const month = d.toLocaleDateString(bg() ? 'bg-BG' : 'en-GB', { month: 'long' });

        return `${d.getDate()} ${month} ${d.getFullYear()}`;
    }

    // Where a rule came from, in a sentence.
    function origin(r) {
        const date = when(r.created_at);

        if (r.source && r.source.trim().replace(/[.!]+$/, '') === r.text.trim().replace(/[.!]+$/, '')) {
            return T('rules.saidOn', { date });
        }

        if (r.source) return T('rules.from', { date });
        if (r.conversation_id) return T('rules.fromChat', { date });

        return T('rules.fromPage', { date });
    }

    // What it does to an action, and what it is about, as short lines.
    function describe(r, into) {
        const lines = [];

        lines.push(T(`rules.mode.${r.mode}`));

        if (r.actions && r.actions.length) {
            lines.push(`${T('rules.about')}: ${r.actions.map((a) => T(`rules.act.${a}`)).join(', ')}`);
        }

        if (r.words && r.words.length) lines.push(`${T('rules.onlyWhen')}: ${r.words.join(', ')}`);
        if (r.days && r.days.length) lines.push(`${T('rules.days')}: ${r.days.map(day).join(', ')}`);

        if (r.kind === 'never' && r.scope && !r.scope.precise && r.actions && r.actions.length) {
            lines.push(T('rules.unsure', { words: (r.scope.unsure || []).join(', ') }));
        }

        for (const line of lines) {
            const p = document.createElement('small');
            p.className = 'study-stats rules-line';
            p.textContent = line;
            into.append(p);
        }
    }

    function quote(r, into) {
        const p = document.createElement('small');
        p.className = 'study-stats rules-origin';
        p.textContent = origin(r);
        into.append(p);

        // The message itself only when it says more than the rule does.
        if (r.source && r.source.trim().replace(/[.!]+$/, '') !== r.text.trim().replace(/[.!]+$/, '')) {
            const q = document.createElement('blockquote');
            q.className = 'rules-quote';
            q.textContent = r.source;
            into.append(q);
        }
    }

    function button(key, onClick, extra) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = `model-action${extra ? ` ${extra}` : ''}`;
        b.textContent = T(key);
        b.addEventListener('click', onClick);

        return b;
    }

    // The editor: words, kind, kinds of action, narrowing words and days.
    function editor(r, onSave, onCancel, saveKey) {
        const form = document.createElement('div');
        form.className = 'rules-editor';

        const textLabel = document.createElement('label');
        textLabel.className = 'field';
        const textSpan = document.createElement('span');
        textSpan.textContent = T('rules.text');
        const text = document.createElement('textarea');
        text.className = 'writing-text';
        text.rows = 2;
        text.maxLength = 400;
        text.value = r.text;
        textLabel.append(textSpan, text);

        const kindLabel = document.createElement('label');
        kindLabel.className = 'field';
        const kindSpan = document.createElement('span');
        kindSpan.textContent = T('rules.kind');
        const kind = document.createElement('select');
        for (const k of (data && data.kinds) || ['always', 'never', 'ask']) {
            const o = document.createElement('option');
            o.value = k;
            o.textContent = T(`rules.kind.${k}`);
            kind.append(o);
        }
        kind.value = r.kind;
        kindLabel.append(kindSpan, kind);

        const acts = document.createElement('fieldset');
        acts.className = 'rules-checks';
        const actsLegend = document.createElement('legend');
        actsLegend.textContent = T('rules.about');
        acts.append(actsLegend);
        for (const a of (data && data.actions) || []) {
            const l = document.createElement('label');
            const box = document.createElement('input');
            box.type = 'checkbox';
            box.value = a;
            box.checked = (r.actions || []).includes(a);
            l.append(box, document.createTextNode(` ${T(`rules.act.${a}`)}`));
            acts.append(l);
        }

        const wordsLabel = document.createElement('label');
        wordsLabel.className = 'field';
        const wordsSpan = document.createElement('span');
        wordsSpan.textContent = T('rules.onlyWhen');
        const words = document.createElement('input');
        words.type = 'text';
        words.maxLength = 600;
        words.placeholder = T('rules.wordsHint');
        words.value = (r.words || []).join(', ');
        wordsLabel.append(wordsSpan, words);

        const days = document.createElement('fieldset');
        days.className = 'rules-checks';
        const daysLegend = document.createElement('legend');
        daysLegend.textContent = T('rules.days');
        days.append(daysLegend);
        for (const d of [1, 2, 3, 4, 5, 6, 0]) {
            const l = document.createElement('label');
            const box = document.createElement('input');
            box.type = 'checkbox';
            box.value = String(d);
            box.checked = (r.days || []).includes(d);
            l.append(box, document.createTextNode(` ${day(d)}`));
            days.append(l);
        }

        const actions = document.createElement('div');
        actions.className = 'writing-actions';

        const values = () => ({
            text: text.value,
            kind: kind.value,
            actions: [...acts.querySelectorAll('input:checked')].map((b) => b.value),
            words: words.value.split(',').map((w) => w.trim()).filter(Boolean),
            days: [...days.querySelectorAll('input:checked')].map((b) => parseInt(b.value, 10)),
        });

        actions.append(button(saveKey || 'rules.save', () => onSave(values())));
        if (onCancel) actions.append(button('rules.cancel', onCancel));

        form.append(textLabel, kindLabel, acts, wordsLabel, days, actions);

        return form;
    }

    function proposalItem(r) {
        const li = document.createElement('li');
        li.className = 'writing-item study-item rules-item is-proposed';

        const main = document.createElement('div');
        main.className = 'study-item-main';

        const strong = document.createElement('strong');
        strong.textContent = r.text;
        main.append(strong);
        describe(r, main);
        quote(r, main);

        const row = document.createElement('div');
        row.className = 'writing-actions';

        row.append(
            button('rules.keep', () => decide(r, true)),
            button('rules.edit', () => { editing = r.id; draw(); }),
            button('rules.decline', () => decide(r, false)),
        );

        main.append(row);

        if (editing === r.id) {
            main.append(editor(r, (v) => decide(r, true, v), () => { editing = 0; draw(); }, 'rules.keep'));
        }

        li.append(main);

        return li;
    }

    function ruleItem(r) {
        const li = document.createElement('li');
        li.className = `writing-item study-item rules-item${r.on ? '' : ' is-off'}`;

        const main = document.createElement('div');
        main.className = 'study-item-main';

        const strong = document.createElement('strong');
        strong.textContent = r.text;

        main.append(strong);
        describe(r, main);
        quote(r, main);

        const row = document.createElement('div');
        row.className = 'writing-actions';

        const remove = button('rules.delete', async () => {
            // Asked once more here, on the page: never a dialog.
            if (!remove.dataset.sure) {
                remove.dataset.sure = '1';
                remove.textContent = T('rules.deleteSure');

                return;
            }

            try {
                await call('DELETE', `/api/rules/${r.id}`);
                say('rules-said', T('rules.deleted'));
                await load(true);
            } catch (err) {
                say('rules-said', T('writing.failed', { why: err.message }), true);
            }
        });

        row.append(button('rules.edit', () => { editing = editing === r.id ? 0 : r.id; draw(); }), remove);
        main.append(row);

        if (editing === r.id) {
            main.append(editor(r, async (v) => {
                try {
                    await call('POST', `/api/rules/${r.id}`, v);
                    editing = 0;
                    say('rules-said', T('rules.saved'));
                    await load(true);
                } catch (err) {
                    say('rules-said', T('writing.failed', { why: err.message }), true);
                }
            }, () => { editing = 0; draw(); }));
        }

        const toggle = document.createElement('label');
        toggle.className = 'sk-toggle';

        const box = document.createElement('input');
        box.type = 'checkbox';
        box.checked = r.on;
        box.setAttribute('aria-label', T('rules.onFor', { text: r.text }));
        box.addEventListener('change', async () => {
            box.disabled = true;

            try {
                await call('POST', `/api/rules/${r.id}/switch`, { on: box.checked });
                r.on = box.checked;
                li.classList.toggle('is-off', !r.on);
                say('rules-said', T(r.on ? 'rules.nowOn' : 'rules.nowOff'));
                count();
            } catch (err) {
                box.checked = !box.checked;
                say('rules-said', T('writing.failed', { why: err.message }), true);
            } finally {
                box.disabled = false;
            }
        });

        const word = document.createElement('span');
        word.textContent = T('rules.on');
        toggle.append(box, word);

        li.append(main, toggle);

        return li;
    }

    async function decide(r, keep, edited) {
        try {
            const out = await call('POST', `/api/rules/${r.id}/decide`, { keep, ...(edited || {}) });
            editing = 0;
            say('rules-said', keep ? T('rules.kept', { text: out.text }) : T('rules.declined'));
            await load(true);

            return out;
        } catch (err) {
            say('rules-said', T('writing.failed', { why: err.message }), true);

            throw err;
        }
    }

    function count() {
        const kept = (data.rules || []).filter((r) => r.status === 'kept');
        el('rules-count').textContent = T('rules.count', { n: kept.length, on: kept.filter((r) => r.on).length });
    }

    function draw() {
        if (!data) return;

        const waiting = (data.rules || []).filter((r) => r.status === 'proposed');
        const kept = (data.rules || []).filter((r) => r.status === 'kept');

        el('rules-waiting').replaceChildren(...waiting.map(proposalItem));
        el('rules-waiting-box').hidden = waiting.length === 0;
        el('rules-list').replaceChildren(...kept.map(ruleItem));
        el('rules-empty').hidden = kept.length > 0;

        count();
    }

    async function load(quiet) {
        try {
            data = await call('GET', '/api/rules');
            draw();
            if (!quiet) say('rules-said', '');
        } catch (err) {
            say('rules-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function add() {
        const text = el('rules-new').value.trim();

        if (!text) {
            say('rules-add-said', T('rules.needWords'), true);

            return;
        }

        try {
            await call('POST', '/api/rules', { text });
            el('rules-new').value = '';
            say('rules-add-said', T('rules.added'));
            await load(true);
        } catch (err) {
            say('rules-add-said', T('writing.failed', { why: err.message }), true);
        }
    }

    /*
     * The card under an answer: the rule as it was noticed, where it came
     * from, and keep / change on the Rules page / not a rule. Added to the
     * transcript after the answer, never instead of it.
     */
    function card(r) {
        const transcript = el('transcript');
        if (!transcript || !r) return;

        const wrap = document.createElement('div');
        wrap.className = 'msg brain rules-card';

        const title = document.createElement('div');
        title.className = 'who';
        title.textContent = T('rules.card.title');

        const text = document.createElement('div');
        text.className = 'body';
        text.textContent = r.text;

        const what = document.createElement('div');
        describe(r, what);

        const said = document.createElement('p');
        said.className = 'note rules-card-said';
        said.setAttribute('role', 'status');

        const row = document.createElement('div');
        row.className = 'writing-actions';

        const done = (key, vars) => {
            row.remove();
            said.textContent = T(key, vars);
        };

        row.append(
            button('rules.keep', async () => {
                try {
                    await call('POST', `/api/rules/${r.id}/decide`, { keep: true });
                    done('rules.keptHere');
                } catch (err) {
                    said.textContent = T('writing.failed', { why: err.message });
                }
            }),
            button('rules.card.open', () => {
                const nav = document.querySelector('.nav-item[data-view="rules"]');
                editing = r.id;
                if (nav) nav.click();
            }),
            button('rules.decline', async () => {
                try {
                    await call('POST', `/api/rules/${r.id}/decide`, { keep: false });
                    done('rules.declined');
                } catch (err) {
                    said.textContent = T('writing.failed', { why: err.message });
                }
            }),
        );

        wrap.append(title, text, what, row, said);
        transcript.appendChild(wrap);
        transcript.scrollTop = transcript.scrollHeight;
    }

    function wire() {
        window.pnRuleCard = card;

        if (!el('rules-list')) return;

        el('rules-add').addEventListener('click', add);

        for (const item of document.querySelectorAll('.nav-item[data-view="rules"]')) {
            item.addEventListener('click', () => load());
        }

        window.addEventListener('pn-language', () => { if (data) draw(); });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
