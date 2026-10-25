/*
 * The mail desk.
 *
 * New mail, sorted by the program from its headers: suspicious first, then
 * what needs a reply, then people, then newsletters by sender. Nothing is
 * opened to sort it. "Draft a reply" hands the request to the conversation,
 * where the assistant reads the message and writes the draft.
 *
 * Drafts wait here for one press of Send each — that press is the yes for
 * that message, as it stands on the screen. Changing the text first saves it;
 * Send and Discard each ask a second press, so neither happens by accident.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => window.pnLang && window.pnLang() === 'bg';

    let canSend = false;
    let lastDigest = null;

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

        if (!res.ok) throw new Error(data.error || `${res.status}`);

        return data.data !== undefined ? data.data : data;
    }

    function say(id, text, bad) {
        const p = el(id);
        if (!p) return;

        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    function when(iso) {
        const at = new Date(iso);
        if (Number.isNaN(at.getTime()) || at.getFullYear() < 2000) return '';

        try {
            return at.toLocaleString(bg() ? 'bg-BG' : 'en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
        } catch {
            return at.toISOString().slice(0, 16).replace('T', ' ');
        }
    }

    function nameOf(from) {
        const m = String(from || '').match(/^\s*"?([^"<]+?)"?\s*<[^>]+>\s*$/);

        return m ? m[1] : String(from || '');
    }

    // Puts a request in the conversation and goes there.
    function askInChat(text) {
        const input = el('input');
        if (!input) return;

        document.querySelector('.nav-item[data-view="console"]')?.click();
        input.value = text;
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.focus();
    }

    function messageRow(item, kind) {
        const li = document.createElement('li');
        li.className = 'writing-item mail-item' + ({ suspicious: ' is-warn', reply: ' is-sure', people: ' is-plain' }[kind] || '');

        const text = document.createElement('div');
        text.className = 'writing-pick';

        const top = document.createElement('span');
        top.className = 'writing-what';

        const who = document.createElement('strong');
        who.textContent = nameOf(item.from);

        const subject = document.createElement('span');
        subject.textContent = item.subject || T('mail.noSubject');

        const at = document.createElement('span');
        at.className = 'mail-when';
        at.textContent = when(item.when);

        top.append(who, subject, at);

        if (!item.seen) {
            const dot = document.createElement('span');
            dot.className = 'mail-new';
            dot.textContent = T('mail.unread');
            top.append(dot);
        }

        const why = document.createElement('span');
        why.className = 'writing-why';
        why.textContent = bg() ? item.why_bg : item.why;

        text.append(top, why);
        li.append(text);

        if (kind === 'reply' || kind === 'people') {
            const draft = document.createElement('button');
            draft.type = 'button';
            draft.className = 'model-action';
            draft.textContent = T('mail.draftIt');
            draft.addEventListener('click', () => askInChat(T('mail.draftAsk', { ref: item.ref, who: nameOf(item.from) })));
            li.append(draft);
        }

        return li;
    }

    function drawDigest(data) {
        const box = el('mail-groups');
        box.replaceChildren();

        if (data.none) {
            say('mail-said', T('mail.none'), true);
            el('mail-counts').textContent = '';
            return;
        }

        const d = data.digest;
        lastDigest = data;

        el('mail-counts').textContent = T('mail.counts', {
            total: d.total, unseen: d.unseen, source: T(data.source === 'server' ? 'mail.fromServer' : 'mail.fromLocal'),
        });
        say('mail-said', d.total ? '' : T('mail.nothing'));

        const groups = [
            ['suspicious', 'mail.suspicious', d.suspicious],
            ['reply', 'mail.reply', d.needs_reply],
            ['people', 'mail.people', d.from_people],
        ];

        for (const [kind, title, list] of groups) {
            if (!list || !list.length) continue;

            const h = document.createElement('h3');
            h.className = 'tr-sub';
            h.textContent = `${T(title)} · ${list.length}`;

            const ol = document.createElement('ul');
            ol.className = 'writing-list';
            list.forEach((item) => ol.append(messageRow(item, kind)));

            box.append(h, ol);
        }

        if (d.bulk && d.bulk.length) {
            const counts = {};
            const order = [];

            for (const item of d.bulk) {
                const n = nameOf(item.from);
                if (!counts[n]) order.push(n);
                counts[n] = (counts[n] || 0) + 1;
            }

            order.sort((a, b) => counts[b] - counts[a]);

            const h = document.createElement('h3');
            h.className = 'tr-sub';
            h.textContent = `${T('mail.bulk')} · ${d.bulk.length}`;

            const p = document.createElement('p');
            p.className = 'note';
            p.textContent = order.slice(0, 12).map((n) => (counts[n] > 1 ? `${n} (${counts[n]})` : n)).join(', ');

            box.append(h, p);
        }
    }

    async function loadDigest() {
        say('mail-said', T('mail.reading'));

        try {
            drawDigest(await get(`/api/mail/digest?hours=${encodeURIComponent(el('mail-hours').value)}`));
        } catch (err) {
            say('mail-said', T('writing.failed', { why: err.message }), true);
        }
    }

    // A button that does its work only on a second press within four seconds.
    function twoStep(button, armed, act) {
        const plain = button.textContent;

        button.addEventListener('click', async () => {
            if (!button.classList.contains('danger')) {
                button.classList.add('danger');
                button.textContent = armed();
                setTimeout(() => {
                    if (!button.disabled) {
                        button.classList.remove('danger');
                        button.textContent = plain;
                    }
                }, 4000);
                return;
            }

            button.disabled = true;

            try {
                await act();
            } finally {
                button.disabled = false;
                button.classList.remove('danger');
                button.textContent = plain;
            }
        });
    }

    function field(labelKey, value, multiline) {
        const label = document.createElement('label');
        label.className = 'field';

        const span = document.createElement('span');
        span.textContent = T(labelKey);

        const input = document.createElement(multiline ? 'textarea' : 'input');
        if (multiline) {
            input.rows = 7;
            input.className = 'writing-text';
        } else {
            input.type = 'text';
        }

        input.value = value || '';
        input.spellcheck = multiline;

        label.append(span, input);

        return [label, input];
    }

    function draftCard(d) {
        const card = document.createElement('div');
        card.className = 'mail-draft' + (d.warning ? ' is-warn' : '');

        const head = document.createElement('p');
        head.className = 'deeds-counts';
        head.textContent = T('mail.draftHead', { id: d.id, at: when(d.created_at) })
            + (d.answers ? ' · ' + T('mail.answers', { what: d.answers }) : '');
        card.append(head);

        if (d.warning) {
            const warn = document.createElement('p');
            warn.className = 'mail-warning';
            warn.textContent = T('mail.warning', { what: d.warning });
            card.append(warn);
        }

        if (d.status === 'failed' && d.note) {
            const failed = document.createElement('p');
            failed.className = 'mail-warning';
            failed.textContent = T('mail.failedNote', { why: d.note });
            card.append(failed);
        }

        const [toLabel, to] = field('mail.to', d.to);
        const [subjectLabel, subject] = field('mail.subject', d.subject);
        const [bodyLabel, body] = field('mail.body', d.body, true);

        card.append(toLabel, subjectLabel, bodyLabel);

        const said = document.createElement('p');
        said.className = 'note';
        said.setAttribute('role', 'status');

        const actions = document.createElement('div');
        actions.className = 'writing-actions';

        const changed = () => to.value !== d.to || subject.value !== d.subject || body.value !== d.body;

        async function save() {
            if (!changed()) return;

            const saved = await send(`/api/mail/drafts/${d.id}`, { to: to.value, subject: subject.value, body: body.value });
            Object.assign(d, saved);
        }

        const saveButton = document.createElement('button');
        saveButton.type = 'button';
        saveButton.className = 'model-action';
        saveButton.textContent = T('mail.save');
        saveButton.addEventListener('click', async () => {
            try {
                await save();
                said.textContent = T('mail.saved');
            } catch (err) {
                said.textContent = T('writing.failed', { why: err.message });
            }
        });

        const sendButton = document.createElement('button');
        sendButton.type = 'button';
        sendButton.className = 'model-action mail-send';
        sendButton.textContent = T('mail.send');
        sendButton.disabled = !canSend;
        twoStep(sendButton, () => T('mail.sendSure', { to: to.value }), async () => {
            try {
                await save();
                await send(`/api/mail/send/${d.id}`);
                said.textContent = T('mail.sent', { to: d.to });
                await loadDrafts();
            } catch (err) {
                said.textContent = T('writing.failed', { why: err.message });
            }
        });

        const discard = document.createElement('button');
        discard.type = 'button';
        discard.className = 'model-action';
        discard.textContent = T('mail.discard');
        twoStep(discard, () => T('gl.sure'), async () => {
            try {
                await send(`/api/mail/drafts/${d.id}/discard`);
                await loadDrafts();
            } catch (err) {
                said.textContent = T('writing.failed', { why: err.message });
            }
        });

        const copy = document.createElement('button');
        copy.type = 'button';
        copy.className = 'model-action';
        copy.textContent = T('writing.copy');
        copy.addEventListener('click', async () => {
            try {
                await navigator.clipboard.writeText(body.value);
                said.textContent = T('writing.copied');
            } catch {
                body.select();
            }
        });

        actions.append(sendButton, saveButton, copy, discard);
        card.append(actions, said);

        return card;
    }

    async function loadDrafts() {
        try {
            const data = await get('/api/mail/drafts');

            canSend = !!data.can_send;
            el('mail-morning').checked = !!data.morning;
            el('mail-nosend').hidden = canSend || data.waiting.length === 0;

            const list = el('mail-drafts');
            list.replaceChildren(...data.waiting.map(draftCard));
            el('mail-nodrafts').hidden = data.waiting.length > 0;

            const count = el('nav-mail');
            if (count) {
                count.textContent = String(data.waiting.length);
                count.hidden = data.waiting.length === 0;
            }

            const done = el('mail-done');
            done.replaceChildren();

            for (const d of data.done.slice(0, 30)) {
                const li = document.createElement('li');
                li.className = 'writing-item' + (d.status === 'sent' ? ' is-sure' : '');

                const span = document.createElement('span');
                span.className = 'writing-why';
                span.textContent = `${T(d.status === 'sent' ? 'mail.wasSent' : 'mail.wasDiscarded')} · ${when(d.sent_at || d.updated_at)} · ${d.to} · ${d.subject}`;

                li.append(span);
                done.append(li);
            }

            el('mail-done-box').hidden = data.done.length === 0;
        } catch (err) {
            say('mail-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('mail-groups')) return;

        el('mail-refresh').addEventListener('click', () => { loadDigest(); loadDrafts(); });
        el('mail-hours').addEventListener('change', loadDigest);

        el('mail-morning').addEventListener('change', async (e) => {
            const box = e.target;
            box.disabled = true;

            try {
                const res = await send('/api/settings/mail-morning', { on: box.checked });
                box.checked = !!res.morning;
                say('mail-said', T(box.checked ? 'mail.morningOn' : 'mail.morningOff'));
            } catch (err) {
                box.checked = !box.checked;
                say('mail-said', T('writing.failed', { why: err.message }), true);
            } finally {
                box.disabled = false;
            }
        });

        for (const item of document.querySelectorAll('.nav-item[data-view="mail"]')) {
            item.addEventListener('click', () => { loadDigest(); loadDrafts(); });
        }

        window.addEventListener('pn-language', () => {
            if (el('mail-groups').closest('[hidden]')) return;

            if (lastDigest) drawDigest(lastDigest);
            loadDrafts();
        });

        // The count of drafts waiting is worth seeing from anywhere.
        loadDrafts();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
