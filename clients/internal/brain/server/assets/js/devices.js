/*
 * The things in the house.
 *
 * The token is treated the way the model keys are — never shown, only whether
 * one is set and its last four characters. It is a key to somebody's front
 * door lock as often as to their lamps.
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
            data = await call('/api/devices');
        } catch (err) {
            return;
        }

        const url = el('ha-url');
        const token = el('ha-token');

        // Never redrawn under somebody's hands: a poll that rewrites a field
        // takes the token out from under the person pasting it.
        if (url && document.activeElement !== url) url.value = data.url || '';

        if (token && document.activeElement !== token) {
            token.placeholder = data.has_token
                ? `token set, ending ${data.token_tail}`
                : 'long-lived access token';
        }

        const why = el('devices-why');

        if (why) {
            why.textContent = data.why
                || `Connected. ${data.devices.length} things.`;
        }

        const forget = el('ha-forget');

        if (forget) forget.hidden = !data.has_token && !data.url;

        const save = el('ha-save');

        if (save) save.textContent = data.connected ? 'Save' : 'Connect';

        draw(data.devices || [], data.connected);
    }

    function draw(devices, connected) {
        const card = el('devices-card');
        const host = el('devices-list');

        if (!card || !host) return;

        card.hidden = !connected;
        host.textContent = '';

        const count = el('devices-count');

        if (count) {
            count.textContent = devices.length;
            count.hidden = devices.length === 0;
        }

        /*
         * Grouped by kind, because a house is fifty things and a flat list of
         * fifty is not something anybody reads. Lights before sensors: one is
         * a thing you press and the other is a thing you glance at.
         */
        const kinds = {};

        for (const d of devices) (kinds[d.domain] = kinds[d.domain] || []).push(d);

        const order = ['light', 'switch', 'climate', 'lock', 'cover', 'media_player'];
        const names = Object.keys(kinds).sort((a, b) => {
            const ai = order.indexOf(a);
            const bi = order.indexOf(b);

            return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi) || a.localeCompare(b);
        });

        for (const kind of names) {
            const title = document.createElement('p');
            title.className = 'insight-note';
            title.textContent = kind.replace(/_/g, ' ');
            host.appendChild(title);

            for (const d of kinds[kind]) host.appendChild(row(d, kind));
        }
    }

    function row(d, kind) {
        const item = document.createElement('div');
        item.className = 'permit';

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name';
        name.textContent = d.name || d.id;

        const state = document.createElement('span');
        state.className = 'permit-what';
        state.textContent = d.unit ? `${d.state} ${d.unit}` : d.state;

        text.append(name, state);
        item.appendChild(text);

        /*
         * A switch only for the things that are switches.
         *
         * A temperature sensor has a state and no button, and putting one
         * beside it would be offering something that cannot happen.
         */
        const switchable = ['light', 'switch', 'fan', 'input_boolean'].includes(kind);

        if (!switchable) {
            const reading = document.createElement('span');
            reading.className = 'permit-reading';
            reading.textContent = 'reading only';
            item.appendChild(reading);

            return item;
        }

        const button = document.createElement('button');
        button.className = 'model-action';
        button.textContent = d.state === 'on' ? 'Turn off' : 'Turn on';

        button.onclick = async () => {
            const wanted = d.state === 'on' ? 'off' : 'on';
            const was = button.textContent;

            button.disabled = true;
            button.textContent = 'one moment…';

            try {
                await call('/api/devices/set', { id: d.id, state: wanted });
                d.state = wanted;
                state.textContent = wanted;
                button.textContent = wanted === 'on' ? 'Turn off' : 'Turn on';
            } catch (err) {
                button.textContent = String(err.message || err).slice(0, 40);
                setTimeout(() => { button.textContent = was; }, 2200);
            }

            button.disabled = false;
        };

        item.appendChild(button);

        return item;
    }

    const save = el('ha-save');

    if (save) {
        save.onclick = async () => {
            const payload = { url: el('ha-url').value };
            const token = el('ha-token').value.trim();

            // An empty box means "leave it alone", never "disconnect".
            if (token !== '') payload.token = token;

            save.disabled = true;

            try {
                await call('/api/devices/connect', payload);
                el('ha-token').value = '';
                await load();
            } catch (err) {
                const why = el('devices-why');

                if (why) why.textContent = String(err.message || err);
            }

            save.disabled = false;
        };
    }

    const forget = el('ha-forget');

    if (forget) {
        forget.onclick = async () => {
            try {
                await call('/api/devices/connect', { forget: true });
                await load();
            } catch (err) {
                /* the page reloads either way */
            }
        };
    }

    for (const item of document.querySelectorAll('[data-view="devices"]')) {
        item.addEventListener('click', () => load());
    }

    load();
}());
