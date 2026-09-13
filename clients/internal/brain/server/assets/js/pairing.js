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
        name.textContent = label;
        wrap.appendChild(name);

        const input = document.createElement('input');
        input.type = 'text';
        input.id = id;
        input.autocomplete = 'off';
        input.placeholder = placeholder;
        wrap.appendChild(input);

        return wrap;
    }

    function show() {
        const screen = document.createElement('div');
        screen.className = 'pairing';

        const card = document.createElement('div');
        card.className = 'card';

        const title = document.createElement('h2');
        title.textContent = 'This device is not paired';
        card.appendChild(title);

        const why = document.createElement('p');
        why.className = 'note';
        why.textContent = 'On the computer your assistant runs on, open Reaching it and '
            + 'press "Pair a device". A code will appear. Type it here.';
        card.appendChild(why);

        card.appendChild(field('What is this device called?', 'claim-name', 'my phone'));
        card.appendChild(field('The code on the computer', 'claim-code', 'ABC 234'));

        const go = document.createElement('button');
        go.type = 'button';
        go.className = 'task-button';
        go.textContent = 'Pair it';
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
                said.textContent = 'Type the code shown on the computer.';
                return;
            }

            said.textContent = 'Pairing…';

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
                    said.textContent = body.error || 'That did not work.';
                    return;
                }

                said.textContent = 'Paired. Opening…';
                location.reload();
            } catch (err) {
                said.textContent = 'That did not work: ' + err.message;
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
