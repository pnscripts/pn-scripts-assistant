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
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

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
        return can === 'full' ? T('reach.canFull') : T('reach.canLimitedNow');
    }

    function whenWords(at) {
        if (!at || at.startsWith('0001')) return T('reach.neverSince');

        const days = Math.round((Date.now() - new Date(at)) / 86400000);

        if (days <= 0) return T('reach.today');
        if (days === 1) return T('reach.yesterday');

        return T('reach.daysAgo', { n: days });
    }

    // The time a code has left, in words, from when it runs out.
    function leftWords(until) {
        const at = until ? new Date(until).getTime() : NaN;

        if (Number.isNaN(at) || at < Date.UTC(2000, 0, 1)) return T('reach.leftFew');

        const minutes = Math.round((at - Date.now()) / 60000);

        if (at - Date.now() < 60000) return T('reach.leftUnder');
        if (minutes <= 1) return T('reach.leftOne');

        return T('reach.leftMany', { n: minutes });
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
        what.textContent = canWords(device.can) + ' · ' + (device.from
            ? T('reach.lastHeardAt', { when: whenWords(device.last_seen), from: device.from })
            : T('reach.lastHeard', { when: whenWords(device.last_seen) }));
        row.appendChild(what);

        // A phone left in a taxi is the case this exists for, so it is one
        // click and it takes effect at once.
        const forget = document.createElement('button');
        forget.type = 'button';
        forget.className = 'task-button';
        forget.textContent = T('reach.unpair');
        forget.addEventListener('click', () => unpair(device.id, device.name));
        row.appendChild(forget);

        return row;
    }

    async function unpair(id, name) {
        say('paired-said', T('reach.unpairing', { name }));

        try {
            const res = await fetch('/api/paired/' + encodeURIComponent(id), { method: 'DELETE' });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say('paired-said', body.error ? T('reach.failedWhy', { why: body.error }) : T('reach.failed'));
                return;
            }

            say('paired-said', T('reach.unpaired', { name }));
            load();
        } catch (err) {
            say('paired-said', T('reach.failedWhy', { why: err.message }));
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
                say('paired-said', body.error ? T('reach.failedWhy', { why: body.error }) : T('reach.failed'));
                return;
            }

            showCode(body.code, body.until);
        } catch (err) {
            say('paired-said', T('reach.failedWhy', { why: err.message }));
        }
    }

    function showCode(code, until) {
        const box = el('pair-code-box');
        if (!box) return;

        box.hidden = !code;

        // Spaced, because it is read across a room. A run of six characters is
        // read wrongly far more often than three and three.
        say('pair-code', code ? code.slice(0, 3) + ' ' + code.slice(3) : '');

        say('pair-how', code ? T('reach.howCode', { left: leftWords(until) }) : '');
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

        say('reach-said', T('reach.saving'));

        try {
            const res = await fetch('/api/reach', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ reach: wanted })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('reach-said', body.error ? T('reach.notSavedWhy', { why: body.error }) : T('reach.notSaved'));
                return;
            }

            say('reach-said', T(body.reach === 'network' ? 'reach.saidNetwork' : 'reach.saidHere'));
            load();
        } catch (err) {
            say('reach-said', T('reach.notSavedWhy', { why: err.message }));
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
                p.textContent = T('reach.noneLong');
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
        showCode(body.code || '', body.code_until || '');
    }

    /* ---------- from outside the house ---------- */

    // What the tunnel still needs, by the name the server gives it.
    const MISSING = {
        setup: 'reach.missing.setup',
        endpoint: 'reach.missing.endpoint',
        devices: 'reach.missing.devices',
    };

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
                state.textContent = T('reach.noWireguard');
            } else if (!body.started) {
                state.textContent = T('reach.notSetUp');
            } else if (body.missing) {
                const key = MISSING[body.missing_what];

                state.textContent = T('reach.nearly', { what: key ? T(key) : body.missing });
            } else {
                state.textContent = T(body.up ? 'reach.on' : 'reach.offReady');
            }
        }

        const endpoint = el('tunnel-endpoint');
        if (endpoint && document.activeElement !== endpoint) endpoint.value = body.endpoint || '';

        const ready = el('tunnel-ready');
        if (ready) ready.hidden = !body.started || !!body.missing;

        // The one thing only its owner can do, said plainly rather than left
        // to be discovered when nothing connects.
        say('tunnel-forward', body.forward ? T('reach.forward', { port: body.port }) : '');

        say('tunnel-inside', body.inside ? T('reach.inside', { address: body.inside }) : '');

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
                what.textContent = T('reach.noWay');
                row.appendChild(what);
                row.appendChild(button(T('reach.giveWay'), () => giveWayIn(device.id)));
            } else if (!way.collected) {
                what.textContent = T('reach.readyAt', { address: way.address });
                row.appendChild(what);

                /*
                 * Collected on the device, over your own network, because it
                 * was paired here and can already reach this. Nothing has to be
                 * photographed and nothing is typed — and the key exists in one
                 * place afterwards.
                 */
                const how = document.createElement('span');
                how.className = 'agent-note';
                how.textContent = T('reach.collectHow');
                row.appendChild(how);

                row.appendChild(button(T('reach.collect'), () => {
                    window.open('/api/tunnel/' + encodeURIComponent(device.id) + '/config', '_blank');
                    setTimeout(loadTunnel, 1500);
                }));
                row.appendChild(button(T('reach.takeAway'), () => removeWayIn(device.id)));
            } else {
                what.textContent = T('reach.inAt', { address: way.address });
                row.appendChild(what);
                row.appendChild(button(T('reach.takeAway'), () => removeWayIn(device.id)));
            }

            list.appendChild(row);
        }
    }

    // then is the key of what to say once it has worked.
    async function post(url, body, then) {
        say('tunnel-said', T('reach.working'));

        try {
            const res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body || {})
            });

            const answer = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('tunnel-said', answer.error ? T('reach.failedWhy', { why: answer.error }) : T('reach.failed'));
                return;
            }

            say('tunnel-said', then ? T(then) : '');
            loadTunnel();
        } catch (err) {
            say('tunnel-said', T('reach.failedWhy', { why: err.message }));
        }
    }

    function setUpTunnel() {
        post('/api/tunnel', { endpoint: el('tunnel-endpoint')?.value.trim() || '' }, 'reach.setUpDone');
    }

    function giveWayIn(id) {
        post('/api/tunnel/' + encodeURIComponent(id), {}, 'reach.wayReady');
    }

    async function removeWayIn(id) {
        try {
            const res = await fetch('/api/tunnel/' + encodeURIComponent(id), { method: 'DELETE' });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say('tunnel-said', body.error ? T('reach.failedWhy', { why: body.error }) : T('reach.failed'));
                return;
            }

            say('tunnel-said', T('reach.wayRemoved'));
            loadTunnel();
        } catch (err) {
            say('tunnel-said', T('reach.failedWhy', { why: err.message }));
        }
    }

    function wire() {
        el('tunnel-save')?.addEventListener('click', setUpTunnel);
        el('tunnel-up')?.addEventListener('click', () => post('/api/tunnel/run', { up: true }, 'reach.tunnelUp'));
        el('tunnel-down')?.addEventListener('click', () => post('/api/tunnel/run', { up: false }, 'reach.tunnelDown'));

        el('pair-offer')?.addEventListener('click', offer);
        el('pair-stop')?.addEventListener('click', stopOffering);
        el('reach-save')?.addEventListener('click', saveReach);

        load();
        loadTunnel();

        // The rows are written here, so they are written again in the other language.
        window.addEventListener('pn-language', () => {
            load();
            loadTunnel();
        });

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
