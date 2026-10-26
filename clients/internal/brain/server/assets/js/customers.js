/*
 * Customers: a reply to a customer from the business's own answers and
 * documents, checked, then kept as a mail draft that waits for its owner's
 * yes on the Mail page (server/care.go). The answer book and how the
 * business wants to sound are kept here too.
 *
 * A reply is a model's words and a customer's message is a stranger's, so
 * everything is drawn with textContent.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    let last = null; // { message, reply }
    let lastPage = null;

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

    const failedText = (err) => (err.status === 503 ? T('care.noModel') : T('writing.failed', { why: err.message }));

    /* ---------- The reply ---------- */

    function point(kind, label, text) {
        const li = document.createElement('li');
        li.className = `writing-item research-claim care-point is-${kind}`;

        const mark = document.createElement('strong');
        mark.textContent = label;

        const said = document.createElement('span');
        said.className = 'research-claim-text';
        said.textContent = text;

        li.append(mark, said);

        return li;
    }

    function showReply(r, quiet) {
        const card = el('care-reply-card');
        const kind = r.kind || {};
        const bg = document.documentElement.lang === 'bg';

        el('care-reply-meta').textContent = [bg ? kind.bg : kind.en, last && last.model ? T('study.by', { model: last.model }) : '']
            .filter(Boolean).join(' · ');

        const ul = el('care-points');
        ul.replaceChildren();

        for (const g of r.gaps || []) ul.append(point('gap', T('care.gap'), g));
        for (const p of r.promises || []) ul.append(point('promise', T('care.promise'), p));
        for (const u of r.unsupported || []) ul.append(point('unsupported', T('care.unsupported'), u));
        for (const a of r.always || []) ul.append(point('always', T('care.alwaysPoint'), a));

        if (!ul.childElementCount) ul.append(point('ok', T('care.okMark'), T('care.ready')));

        const based = (r.based_on || []).map((p) => p.title);
        el('care-based').textContent = based.length ? T('care.basedOn', { what: based.join('; ') }) : T('care.basedOnNothing');

        if (!quiet) {
            el('care-reply').value = r.text || '';
            say('care-kept', '');
        }

        card.hidden = false;

        if (!quiet) card.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    async function write() {
        const message = el('care-message').value.trim();

        if (!message) { el('care-message').focus(); return; }

        el('care-write').disabled = true;
        say('care-said', T('care.writing'));

        try {
            const data = await send('/api/care/reply', { message, language: el('care-language').value });

            last = { message, reply: data.reply, model: data.reply.model };
            say('care-said', data.ready ? T('care.readyShort') : T('care.look'));
            showReply(data.reply);
        } catch (err) {
            say('care-said', failedText(err), true);
        } finally {
            el('care-write').disabled = false;
        }
    }

    async function keep() {
        const body = el('care-reply').value.trim();
        const to = el('care-to').value.trim();

        if (!body) { el('care-reply').focus(); return; }
        if (!to) { el('care-to').focus(); say('care-kept', T('care.needTo'), true); return; }

        try {
            const data = await send('/api/care/keep', { to, subject: el('care-subject').value.trim(), body });

            say('care-kept', (data.gaps || []).length ? T('care.keptGaps', { n: data.draft.id }) : T('care.kept', { n: data.draft.id }));
        } catch (err) {
            say('care-kept', T('writing.failed', { why: err.message }), true);
        }
    }

    async function copy() {
        try {
            await navigator.clipboard.writeText(el('care-reply').value);
            say('care-kept', T('care.copied'));
        } catch {
            el('care-reply').select();
            say('care-kept', T('care.copyByHand'));
        }
    }

    // The reply as its owner changed it becomes the answer to the
    // customer's question, for next time.
    function learn() {
        if (!last) return;

        el('care-q').value = last.message.split('\n').map((l) => l.trim()).filter(Boolean).join(' ').slice(0, 300);
        // Without the signature: it is put on every reply anyway.
        const sig = ((lastPage && lastPage.profile && lastPage.profile.signature) || '').trim();
        let answer = el('care-reply').value.trim();
        if (sig && answer.endsWith(sig)) answer = answer.slice(0, -sig.length).trim();
        el('care-a').value = answer;
        el('care-q').scrollIntoView({ behavior: 'smooth', block: 'center' });
        el('care-a').focus();
        say('care-book-said', T('care.learnHint'));
    }

    /* ---------- The answer book and the profile ---------- */

    function drawBook(answers) {
        const ul = el('care-list');
        ul.replaceChildren();

        for (const a of answers) {
            const li = document.createElement('li');
            li.className = 'writing-item study-item';

            const main = document.createElement('div');
            main.className = 'study-item-main';

            const q = document.createElement('strong');
            q.textContent = a.question;

            const ans = document.createElement('small');
            ans.className = 'study-stats care-answer';
            ans.textContent = a.answer;

            main.append(q, ans);

            const actions = document.createElement('div');
            actions.className = 'study-item-actions';

            const edit = document.createElement('button');
            edit.type = 'button';
            edit.className = 'model-action quiet';
            edit.textContent = T('care.edit');
            edit.addEventListener('click', () => {
                el('care-q').value = a.question;
                el('care-a').value = a.answer;
                el('care-add').dataset.id = String(a.id);
                el('care-add').textContent = T('care.change');
                el('care-q').focus();
            });

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
                    await send(`/api/care/answers/${a.id}/remove`);
                    await load();
                } catch (err) {
                    say('care-book-said', T('writing.failed', { why: err.message }), true);
                }
            });

            actions.append(edit, remove);
            li.append(main, actions);
            ul.append(li);
        }

        el('care-empty').hidden = answers.length > 0;
    }

    function drawProfile(p, files) {
        el('care-name').value = p.name || '';
        el('care-tone').value = p.tone === 'formal' ? 'formal' : 'friendly';
        el('care-signature').value = p.signature || '';
        el('care-folder').value = p.folder || '';
        el('care-always').value = p.always || '';

        if (p.folder) say('care-profile-said', T('care.files', { n: files }));
    }

    async function load() {
        try {
            const data = await get('/api/care');

            lastPage = data;
            drawBook(data.answers || []);
            drawProfile(data.profile || {}, data.files || 0);
        } catch (err) {
            say('care-book-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function add() {
        const question = el('care-q').value.trim();
        const answer = el('care-a').value.trim();
        const button = el('care-add');

        if (!question) { el('care-q').focus(); return; }
        if (!answer) { el('care-a').focus(); return; }

        try {
            await send('/api/care/answers', { id: Number(button.dataset.id || 0), question, answer });

            el('care-q').value = '';
            el('care-a').value = '';
            delete button.dataset.id;
            button.textContent = T('care.add');
            say('care-book-said', T('care.added'));
            await load();
        } catch (err) {
            say('care-book-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function save() {
        try {
            await send('/api/care/profile', {
                name: el('care-name').value.trim(),
                tone: el('care-tone').value,
                signature: el('care-signature').value.trim(),
                folder: el('care-folder').value.trim(),
                always: el('care-always').value.trim(),
            });

            await load();
            say('care-profile-said', `${T('care.saved')} ${el('care-folder').value.trim() ? T('care.files', { n: lastPage.files || 0 }) : ''}`.trim());
        } catch (err) {
            say('care-profile-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('care-list')) return;

        el('care-write').addEventListener('click', write);
        el('care-keep').addEventListener('click', keep);
        el('care-copy').addEventListener('click', copy);
        el('care-learn').addEventListener('click', learn);
        el('care-add').addEventListener('click', add);
        el('care-save').addEventListener('click', save);

        for (const item of document.querySelectorAll('.nav-item[data-view="customers"]')) {
            item.addEventListener('click', load);
        }

        window.addEventListener('pn-language', () => {
            if (last && !el('care-reply-card').hidden) showReply(last.reply, true);
            if (lastPage) drawBook(lastPage.answers || []);
        });

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
