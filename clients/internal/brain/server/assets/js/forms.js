/*
 * PDF forms: a form's fields laid out to fill in by their kind, and the
 * form filled into a new file beside it, every value read back from the new
 * file first (server/forms.go). Field names and values are somebody else's
 * words, so everything is drawn with textContent.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    let fields = [];
    let opened = '';

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
            err.problems = data.problems || [];
            throw err;
        }

        return data.data !== undefined ? data.data : data;
    }

    function say(id, text, bad) {
        const p = el(id);
        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    const idOf = (i) => `forms-f-${i}`;

    function draw() {
        const box = el('forms-fields');
        box.replaceChildren();

        fields.forEach((f, i) => {
            const label = document.createElement('label');
            label.className = 'field';

            const name = document.createElement('span');
            name.textContent = f.name + (f.required ? ' *' : '');

            let input;

            if (f.kind === 'checkbox') {
                label.classList.add('books-check');
                input = document.createElement('input');
                input.type = 'checkbox';
                input.checked = f.value !== '' && f.value !== 'Off';
                label.append(input, name);
            } else if (f.kind === 'radio' || (f.kind === 'choice' && !f.edit)) {
                input = document.createElement('select');

                const none = document.createElement('option');
                none.value = f.kind === 'radio' ? 'Off' : '';
                none.textContent = T('forms.none');
                input.append(none);

                for (const o of f.options || []) {
                    const opt = document.createElement('option');
                    opt.value = o;
                    opt.textContent = o;
                    input.append(opt);
                }

                input.value = (f.options || []).includes(f.value) ? f.value : none.value;
                label.append(name, input);
            } else {
                input = document.createElement('input');
                input.type = 'text';
                input.value = f.value || '';
                // The limit is shown, not enforced by cutting: a value that is
                // too long is refused with the reason, never shortened unseen.
                if (f.max_len) name.textContent += ` (${T('forms.max', { n: f.max_len })})`;
                if (f.kind === 'choice') input.setAttribute('list', `${idOf(i)}-list`);
                label.append(name, input);
            }

            input.id = idOf(i);

            if (f.read_only || f.kind === 'signature' || f.kind === 'button') {
                input.disabled = true;
                label.title = T(f.kind === 'signature' ? 'forms.signature' : 'forms.readOnly');
            }

            if ((f.value || '').length > 40) label.classList.add('books-wide');

            box.append(label);
        });

        el('forms-count').textContent = T('forms.count', { n: fields.length });
        el('forms-card').hidden = false;
    }

    async function open() {
        const path = el('forms-path').value.trim();

        if (!path) { el('forms-path').focus(); return; }

        say('forms-said', '');
        say('forms-done', '');
        el('forms-problems').replaceChildren();

        try {
            const data = await send('/api/forms/fields', { path });

            fields = data.fields || [];
            opened = path;
            el('forms-out').value = data.out || '';
            draw();
        } catch (err) {
            el('forms-card').hidden = true;
            say('forms-said', err.status === 422 ? err.message : T('writing.failed', { why: err.message }), true);
        }
    }

    function problemText(p) {
        const f = fields.find((x) => x.name === p.field) || {};
        const key = `forms.p.${p.key}`;
        const said = T(key, { field: p.field, max: f.max_len || '', choices: (f.options || []).join(', ') });

        return said !== key ? said : `${p.field}: ${p.why}`;
    }

    function drawProblems(problems) {
        const ul = el('forms-problems');
        ul.replaceChildren();

        for (const p of problems || []) {
            const li = document.createElement('li');
            li.className = 'writing-item research-claim is-flagged';

            const mark = document.createElement('strong');
            mark.textContent = T('tab.lookAt');

            const what = document.createElement('span');
            what.className = 'research-claim-text';
            what.textContent = problemText(p);

            li.append(mark, what);
            ul.append(li);
        }
    }

    async function fill() {
        const values = {};

        fields.forEach((f, i) => {
            const input = el(idOf(i));
            if (!input || input.disabled) return;

            const v = f.kind === 'checkbox' ? (input.checked ? 'yes' : 'no') : input.value;
            const was = f.kind === 'checkbox' ? (f.value !== '' && f.value !== 'Off' ? 'yes' : 'no') : (f.value || '');

            if (v !== was) values[f.name] = v;
        });

        if (!Object.keys(values).length) {
            say('forms-done', T('forms.nothing'), true);
            return;
        }

        el('forms-fill').disabled = true;

        try {
            const data = await send('/api/forms/fill', { path: opened, values, out: el('forms-out').value.trim() });

            say('forms-done', T('forms.saved', { n: (data.set || []).length, path: data.path }));
            drawProblems(data.problems);
        } catch (err) {
            say('forms-done', err.status === 409 ? T('forms.exists') : (err.status === 422 && err.problems.length ? T('forms.notFilled') : err.message), true);
            drawProblems(err.problems);
        } finally {
            el('forms-fill').disabled = false;
        }
    }

    function wire() {
        if (!el('forms-open')) return;

        el('forms-open').addEventListener('click', open);
        el('forms-fill').addEventListener('click', fill);
        el('forms-path').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                open();
            }
        });

        window.addEventListener('pn-language', () => {
            if (fields.length) el('forms-count').textContent = T('forms.count', { n: fields.length });
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
