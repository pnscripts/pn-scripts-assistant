/*
 * Connecting to a model that is not on this machine.
 *
 * The keys have lived in the settings file since the beginning with nowhere to
 * type one, so using a hosted model meant finding that file and knowing its
 * name. That is not a feature this program had; it is a feature its author
 * had.
 *
 * A key is never shown. The page is told whether one is set and its last four
 * characters — enough to answer "is that the one I meant", useless to somebody
 * reading over a shoulder, and it means a screenshot of this page is not a
 * leak.
 */
(function () {
    const el = (id) => document.getElementById(id);

    async function call(path, payload) {
        const res = await fetch(path, {
            method: payload ? 'POST' : 'GET',
            headers: payload
                ? { 'Content-Type': 'application/json', Accept: 'application/json' }
                : { Accept: 'application/json' },
            body: payload ? JSON.stringify(payload) : undefined,
        });

        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `that did not work (${res.status})`);

        return body.data !== undefined ? body.data : body;
    }

    async function load() {
        let data;

        try {
            data = await call('/api/providers');
        } catch (err) {
            return;
        }

        const host = el('providers-list');

        if (!host) return;

        // Nothing is redrawn while somebody is typing into it. A poll that
        // rewrites a field takes the key out from under the person entering it.
        if (host.contains(document.activeElement)) return;

        host.textContent = '';

        for (const p of data.providers || []) host.appendChild(row(p));
    }

    function row(p) {
        const card = document.createElement('div');
        card.className = 'provider';
        card.dataset.state = !p.permitted ? 'forbidden'
            : p.reachable ? 'reachable'
                : (p.needs_key && !p.has_key) ? 'empty' : 'unreachable';

        const head = document.createElement('div');
        head.className = 'provider-head';

        const name = document.createElement('span');
        name.className = 'provider-name';
        name.textContent = p.name;

        const state = document.createElement('span');
        state.className = 'provider-state';

        /*
         * Four states, not two.
         *
         * "No key" and "key that does not work" look identical as a red dot
         * and mean completely different things to do next — and a provider
         * the privacy setting forbids is a third, where nothing is wrong at
         * all. Saying which is most of the value of this page.
         */
        state.textContent = !p.permitted ? 'not allowed at this privacy setting'
            : p.reachable ? 'answering'
                : (p.needs_key && !p.has_key) ? 'no key yet'
                    : 'not answering';

        head.append(name, state);

        if (p.default) {
            const mark = document.createElement('span');
            mark.className = 'provider-default';
            mark.textContent = 'default';
            head.appendChild(mark);
        }

        const what = document.createElement('p');
        what.className = 'provider-what';
        what.textContent = p.what;

        card.append(head, what);

        const fields = document.createElement('div');
        fields.className = 'provider-fields';

        let key = null;

        if (p.needs_key) {
            key = document.createElement('input');
            key.type = 'password';
            key.autocomplete = 'off';
            key.spellcheck = false;
            // The tail, not the key: enough to recognise, useless to steal.
            key.placeholder = p.has_key ? `key set, ending ${p.key_tail}` : 'paste the key';
            fields.appendChild(key);
        }

        const url = p.needs_key ? null : document.createElement('input');

        if (url) {
            url.type = 'text';
            url.value = p.url || '';
            url.placeholder = 'http://127.0.0.1:11434';
            fields.appendChild(url);
        }

        const model = document.createElement('input');
        model.type = 'text';
        model.value = p.model || '';
        model.placeholder = 'model name, or leave empty for the default';
        fields.appendChild(model);

        card.appendChild(fields);

        const actions = document.createElement('div');
        actions.className = 'provider-actions';

        const save = document.createElement('button');
        save.className = 'model-action';
        save.textContent = p.has_key || !p.needs_key ? 'Save' : 'Connect';
        save.onclick = async () => {
            const payload = { provider: p.name, model: model.value };

            // An empty box means "leave it alone", never "delete the key".
            // Those are different intentions and there is a separate button
            // for the second one.
            if (key && key.value.trim() !== '') payload.key = key.value.trim();
            if (url) payload.url = url.value.trim();

            await act(save, () => call('/api/providers/connect', payload));

            if (key) key.value = '';

            load();
        };

        const test = document.createElement('button');
        test.className = 'model-action';
        test.textContent = 'Test';
        test.onclick = () => act(test, async () => {
            const answer = await call(`/api/providers/${p.name}/test`, {});

            state.textContent = answer.why || (answer.reachable ? 'answering' : 'not answering');
            card.dataset.state = answer.reachable ? 'reachable' : 'unreachable';
        });

        actions.append(save, test);

        if (p.needs_key && p.has_key) {
            const forget = document.createElement('button');
            forget.className = 'model-action danger';
            forget.textContent = 'Forget the key';
            forget.onclick = () => act(forget, async () => {
                await call('/api/providers/connect', { provider: p.name, forget: true });
                load();
            });

            actions.appendChild(forget);
        }

        card.appendChild(actions);

        return card;
    }

    /*
     * A button that says what happened to it.
     *
     * Without this a save is a click with no consequence anybody can see, and
     * the honest reading of that is that it did not work.
     */
    async function act(button, work) {
        const was = button.textContent;

        button.disabled = true;
        button.textContent = 'one moment…';

        try {
            await work();
            button.textContent = 'done';
        } catch (err) {
            button.textContent = String(err.message || err).slice(0, 60);
        }

        setTimeout(() => {
            button.textContent = was;
            button.disabled = false;
        }, 2200);
    }

    for (const item of document.querySelectorAll('[data-view="models"]')) {
        item.addEventListener('click', () => load());
    }

    load();
}());
