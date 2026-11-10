/*
 * The screen a device sees before it has been let in.
 *
 * Without this, an unpaired phone loads the whole interface and every single
 * request in it fails quietly — nine panels of dashes and spinners, which
 * reads as a broken program rather than as a locked door. A locked door should
 * look like a locked door.
 *
 * Loaded before everything else and checks once. On this computer the check
 * passes immediately and nothing here ever runs again.
 */
(function () {
    'use strict';

    /*
     * In the language of the page, which on a device not yet let in is the
     * browser's own: the assistant cannot be asked before the code is typed.
     */
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    async function locked() {
        try {
            const res = await fetch('/api/status');

            return res.status === 401;
        } catch (err) {
            // Unreachable is a different problem with a different answer, and
            // the rest of the page says so better than this could.
            return false;
        }
    }

    function field(label, id, placeholder) {
        const wrap = document.createElement('div');
        wrap.className = 'field';

        const name = document.createElement('span');
        name.dataset.i18n = label;
        name.textContent = T(label);
        wrap.appendChild(name);

        const input = document.createElement('input');
        input.type = 'text';
        input.id = id;
        input.autocomplete = 'off';
        input.placeholder = placeholder.includes('.') ? T(placeholder) : placeholder;
        if (placeholder.includes('.')) input.dataset.i18nPlaceholder = placeholder;
        wrap.appendChild(input);

        return wrap;
    }

    function show() {
        const screen = document.createElement('div');
        screen.className = 'pairing';

        const card = document.createElement('div');
        card.className = 'card';

        const title = document.createElement('h2');
        title.dataset.i18n = 'reach.lockedTitle';
        title.textContent = T('reach.lockedTitle');
        card.appendChild(title);

        const why = document.createElement('p');
        why.className = 'note';
        why.dataset.i18n = 'reach.lockedHow';
        why.textContent = T('reach.lockedHow');
        card.appendChild(why);

        card.appendChild(field('reach.deviceName', 'claim-name', 'reach.deviceNameHint'));
        card.appendChild(field('reach.codeHere', 'claim-code', 'ABC 234'));

        const go = document.createElement('button');
        go.type = 'button';
        go.className = 'task-button';
        go.dataset.i18n = 'reach.pairIt';
        go.textContent = T('reach.pairIt');
        card.appendChild(go);

        const said = document.createElement('p');
        said.className = 'note';
        card.appendChild(said);

        screen.appendChild(card);

        document.body.textContent = '';
        document.body.appendChild(screen);

        async function claim() {
            const code = document.getElementById('claim-code').value.trim();

            if (!code) {
                said.textContent = T('reach.typeCode');
                return;
            }

            said.textContent = T('reach.pairing');

            try {
                const res = await fetch('/api/pair/claim', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        code: code,
                        name: document.getElementById('claim-name').value.trim()
                    })
                });

                const body = await res.json().catch(() => ({}));

                if (!res.ok) {
                    said.textContent = body.error ? T('reach.failedWhy', { why: body.error }) : T('reach.failed');
                    return;
                }

                said.textContent = T('reach.paired');
                location.reload();
            } catch (err) {
                said.textContent = T('reach.failedWhy', { why: err.message });
            }
        }

        go.addEventListener('click', claim);

        document.getElementById('claim-code').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') { e.preventDefault(); claim(); }
        });
    }

    async function check() {
        if (await locked()) show();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', check);
    } else {
        check();
    }
})();
