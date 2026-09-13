/*
 * What may reach this assistant, and how something becomes one of them.
 *
 * A different question from privacy, which is about where your words go. This
 * is about who may ask — and until something is paired the answer is nothing
 * at all, not because of a setting but because the program refuses to listen
 * anywhere but this computer.
 */
(function () {
    'use strict';

    const el = (id) => document.getElementById(id);

    function say(id, text) {
        const node = el(id);
        if (node) node.textContent = text || '';
    }

    function button(label, onClick) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'task-button';
        b.textContent = label;
        b.addEventListener('click', onClick);

        return b;
    }

    function canWords(can) {
        return can === 'full' ? 'everything this computer can' : 'talks to it and watches';
    }

    function whenWords(at) {
        if (!at || at.startsWith('0001')) return 'not since it was paired';

        const days = Math.round((Date.now() - new Date(at)) / 86400000);

        if (days <= 0) return 'today';
        if (days === 1) return 'yesterday';

        return days + ' days ago';
    }

    function deviceRow(device) {
        const row = document.createElement('div');
        row.className = 'task-row';

        const name = document.createElement('span');
        name.className = 'agent-title';
        name.textContent = device.name;
        row.appendChild(name);

        const what = document.createElement('span');
        what.className = 'task-state';
        what.textContent = canWords(device.can)
            + ' · last heard from ' + whenWords(device.last_seen)
            + (device.from ? ' at ' + device.from : '');
        row.appendChild(what);

        // A phone left in a taxi is the case this exists for, so it is one
        // click and it takes effect at once.
        const forget = document.createElement('button');
        forget.type = 'button';
        forget.className = 'task-button';
        forget.textContent = 'Unpair it';
        forget.addEventListener('click', () => unpair(device.id, device.name));
        row.appendChild(forget);

        return row;
    }

    async function unpair(id, name) {
        say('paired-said', 'Unpairing ' + name + '…');

        try {
            const res = await fetch('/api/paired/' + encodeURIComponent(id), { method: 'DELETE' });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say('paired-said', body.error || 'That did not work.');
                return;
            }

            say('paired-said', name + ' can no longer reach this. It takes effect now, '
                + 'not at the next restart.');
            load();
        } catch (err) {
            say('paired-said', 'That did not work: ' + err.message);
        }
    }

    async function offer() {
        say('paired-said', '');

        try {
            const res = await fetch('/api/pair/offer', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ can: el('pair-can')?.value || 'limited' })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('paired-said', body.error || 'That did not work.');
                return;
            }

            showCode(body.code, body.for);
        } catch (err) {
            say('paired-said', 'That did not work: ' + err.message);
        }
    }

    function showCode(code, howLong) {
        const box = el('pair-code-box');
        if (!box) return;

        box.hidden = !code;

        // Spaced, because it is read across a room. A run of six characters is
        // read wrongly far more often than three and three.
        say('pair-code', code ? code.slice(0, 3) + ' ' + code.slice(3) : '');

        say('pair-how', code
            ? 'On the device, open the address below and type this in. It lasts '
              + (howLong || 'a few minutes') + ', and works once.'
            : '');
    }

    async function stopOffering() {
        try {
            await fetch('/api/pair/stop', { method: 'POST' });
        } catch (err) {
            /* the next poll will show it either way */
        }

        showCode('', '');
        load();
    }

    async function saveReach() {
        const wanted = el('reach')?.value || 'here';

        say('reach-said', 'Saving…');

        try {
            const res = await fetch('/api/reach', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ reach: wanted })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('reach-said', body.error || 'That did not save.');
                return;
            }

            say('reach-said', body.said || 'Saved.');
            load();
        } catch (err) {
            say('reach-said', 'That did not save: ' + err.message);
        }
    }

    async function load() {
        let body;

        try {
            const res = await fetch('/api/paired');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const list = el('paired-list');

        if (list) {
            list.textContent = '';

            const devices = body.devices || [];

            if (!devices.length) {
                const p = document.createElement('p');
                p.className = 'note';
                p.textContent = 'Nothing is paired, so nothing but this computer can reach it.';
                list.appendChild(p);
            } else {
                for (const device of devices) list.appendChild(deviceRow(device));
            }
        }

        const reach = el('reach');
        if (reach && document.activeElement !== reach) reach.value = body.reach || 'here';

        // Where to go and what to check, shown only once it would work.
        const where = el('reach-where');

        if (where) {
            where.hidden = !body.open;

            const addresses = el('reach-addresses');

            if (addresses) {
                addresses.textContent = '';

                for (const address of (body.addresses || [])) {
                    const line = document.createElement('p');
                    line.className = 'agent-note';
                    line.textContent = address;
                    addresses.appendChild(line);
                }
            }

            say('reach-fingerprint', body.fingerprint || '');
        }

        // The code, if one is being offered — so it survives changing view and
        // stops showing itself the moment it runs out.
        showCode(body.code || '', '');
    }

    /* ---------- from outside the house ---------- */

    /*
     * A tunnel, not a door.
     *
     * The device joins the home network; nothing new is exposed to the
     * internet, and everything above holds unchanged because from the
     * assistant's point of view nothing has changed at all.
     */
    async function loadTunnel() {
        let body;

        try {
            const res = await fetch('/api/tunnel');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const state = el('tunnel-state');

        if (state) {
            if (!body.installed) {
                state.textContent = 'WireGuard is not installed yet. It is in the list of '
                    + 'parts this machine needs, under "What it runs on" — the tunnel itself '
                    + 'is already in your system, this is the tool that configures it.';
            } else if (!body.started) {
                state.textContent = 'Not set up. A tunnel puts a device on your home network '
                    + 'from anywhere, without anything new being reachable from the internet.';
            } else if (body.missing) {
                state.textContent = 'Nearly: ' + body.missing + '.';
            } else {
                state.textContent = body.up
                    ? 'The tunnel is on.'
                    : 'Set up, and off. Turn it on to let your devices in from outside.';
            }
        }

        const endpoint = el('tunnel-endpoint');
        if (endpoint && document.activeElement !== endpoint) endpoint.value = body.endpoint || '';

        const ready = el('tunnel-ready');
        if (ready) ready.hidden = !body.started || !!body.missing;

        // The one thing only its owner can do, said plainly rather than left
        // to be discovered when nothing connects.
        say('tunnel-forward', body.forward
            ? 'Your router must send ' + body.forward + '. That is the one step this '
              + 'program cannot do for you, and nothing gets through without it.'
            : '');

        say('tunnel-inside', body.inside
            ? 'Once a device is through, it opens ' + body.inside
            : '');

        drawTunnelDevices(body);
    }

    function drawTunnelDevices(body) {
        const list = el('tunnel-devices');
        if (!list) return;

        list.textContent = '';

        if (!body.started) return;

        for (const device of (body.devices || [])) {
            const way = (body.ways || {})[device.id];

            const row = document.createElement('div');
            row.className = 'task-row';

            const name = document.createElement('span');
            name.className = 'agent-title';
            name.textContent = device.name;
            row.appendChild(name);

            const what = document.createElement('span');
            what.className = 'task-state';

            if (!way) {
                what.textContent = 'no way in from outside';
                row.appendChild(what);
                row.appendChild(button('Give it one', () => giveWayIn(device.id)));
            } else if (!way.collected) {
                what.textContent = 'ready at ' + way.address + ' — collect it on the device';
                row.appendChild(what);

                /*
                 * Collected on the device, over your own network, because it
                 * was paired here and can already reach this. Nothing has to be
                 * photographed and nothing is typed — and the key exists in one
                 * place afterwards.
                 */
                const how = document.createElement('span');
                how.className = 'agent-note';
                how.textContent = 'On the device, open this page and press Collect. '
                    + 'It can only be taken once.';
                row.appendChild(how);

                row.appendChild(button('Collect it', () => {
                    window.open('/api/tunnel/' + encodeURIComponent(device.id) + '/config', '_blank');
                    setTimeout(loadTunnel, 1500);
                }));
                row.appendChild(button('Take it away', () => removeWayIn(device.id)));
            } else {
                what.textContent = 'in at ' + way.address;
                row.appendChild(what);
                row.appendChild(button('Take it away', () => removeWayIn(device.id)));
            }

            list.appendChild(row);
        }
    }

    async function post(url, body, then) {
        say('tunnel-said', 'Working…');

        try {
            const res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body || {})
            });

            const answer = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('tunnel-said', answer.error || 'That did not work.');
                return;
            }

            say('tunnel-said', answer.said || (then || ''));
            loadTunnel();
        } catch (err) {
            say('tunnel-said', 'That did not work: ' + err.message);
        }
    }

    function setUpTunnel() {
        post('/api/tunnel', { endpoint: el('tunnel-endpoint')?.value.trim() || '' },
            'Set up. Now give each device a way in.');
    }

    function giveWayIn(id) {
        post('/api/tunnel/' + encodeURIComponent(id), {},
            'Ready. Collect it on the device itself, while it is still on your network.');
    }

    async function removeWayIn(id) {
        try {
            const res = await fetch('/api/tunnel/' + encodeURIComponent(id), { method: 'DELETE' });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say('tunnel-said', body.error || 'That did not work.');
                return;
            }

            say('tunnel-said', 'That device can no longer come in from outside.');
            loadTunnel();
        } catch (err) {
            say('tunnel-said', 'That did not work: ' + err.message);
        }
    }

    function wire() {
        el('tunnel-save')?.addEventListener('click', setUpTunnel);
        el('tunnel-up')?.addEventListener('click', () => post('/api/tunnel/run', { up: true }));
        el('tunnel-down')?.addEventListener('click', () => post('/api/tunnel/run', { up: false }));

        el('pair-offer')?.addEventListener('click', offer);
        el('pair-stop')?.addEventListener('click', stopOffering);
        el('reach-save')?.addEventListener('click', saveReach);

        load();
        loadTunnel();

        document.querySelector('.nav-item[data-view="reach"]')?.addEventListener('click', () => {
            load();
            loadTunnel();
        });

        /*
         * Polled only while the view is open, and only because of the code.
         *
         * Everything else here changes when somebody changes it. The code runs
         * out on its own, and a code still on screen after it has stopped
         * working is a code somebody types and is told is wrong.
         */
        setInterval(() => {
            const view = document.querySelector('section.view[data-view="reach"]');

            if (view && !view.hidden) {
                load();
                loadTunnel();
            }
        }, 5000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
