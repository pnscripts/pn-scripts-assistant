/*
 * Settings, and being introduced once.
 *
 * Everything here can also be done by asking out loud — that is the point of
 * the tools behind it. This exists because a minute of a local model thinking
 * is a long way to go to rename something, and anything the brain can change
 * about this program its owner should be able to change without asking it.
 */

/*
 * Wrapped, because these are plain scripts sharing one global scope.
 *
 * Unwrapped, `const el` here collided with the same name already declared at
 * the top of console.js, which is a SyntaxError — and a SyntaxError does not
 * fail the line, it fails the file. Every panel this script fills stayed empty
 * with nothing on screen to say why.
 */
(function () {

const el = (id) => document.getElementById(id);

const COLOUR_PARTS = [
    ['idle', 'At rest', 'Nothing happening'],
    ['listening', 'Hearing you', 'While you are speaking'],
    ['thinking_core', 'Working', 'While it is thinking or using a tool'],
    ['speaking', 'Its own voice', 'While it is speaking'],
];

async function load() {
    let status;

    try {
        status = await fetch('/api/status').then((r) => r.json());
    } catch {
        return;
    }

    /*
     * Asked once, on a machine that has never run this before.
     *
     * A brain that names itself is a brain somebody has to go and look up how
     * to rename. Asking takes ten seconds and the answer is the first thing it
     * will ever be called.
     */
    if (status.first_run) {
        const naming = el('naming');

        if (naming) {
            naming.hidden = false;
            el('naming-name').focus();
        }
    }

    if (el('set-name')) {
        el('set-name').value = status.name || '';
        el('set-owner').value = status.owner || '';
        el('set-wake').value = status.wake_word || '';
        el('set-privacy').value = (status.privacy && status.privacy.mode) || 'private';
    }

    renderColours();
}

async function renderColours() {
    const host = el('colour-rows');

    if (!host) return;

    let look;

    try {
        look = await fetch('/api/appearance').then((r) => r.json());
    } catch {
        return;
    }

    host.innerHTML = '';

    for (const [key, name, when] of COLOUR_PARTS) {
        const row = document.createElement('div');

        row.className = 'row';

        const swatch = document.createElement('span');
        swatch.className = 'row-icon colour-swatch';
        swatch.style.background = look[key];
        swatch.style.borderColor = look[key];

        const text = document.createElement('span');
        text.className = 'row-text';

        const title = document.createElement('span');
        title.className = 'row-name';
        title.textContent = name;

        const note = document.createElement('span');
        note.className = 'row-state';
        note.textContent = `${when} · ${look[key]}`;

        text.append(title, note);

        const pick = document.createElement('input');
        pick.type = 'color';
        pick.className = 'colour-pick';
        pick.value = look[key];
        pick.onchange = () => setColour(key, pick.value);

        row.append(swatch, text, pick);
        host.append(row);
    }
}

// The part names the brain understands, so the panel and the spoken instruction
// reach the same setting rather than two that drift apart.
const SPOKEN_PART = {
    idle: 'idle',
    listening: 'listening',
    thinking_core: 'core',
    speaking: 'speaking',
};

async function setColour(key, value) {
    try {
        await fetch('/api/appearance', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ part: SPOKEN_PART[key], colour: value }),
        });
    } catch {
        /* The swatch simply does not move. */
    }

    renderColours();
}

async function save(values, note) {
    try {
        const saved = await fetch('/api/settings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(values),
        }).then((r) => r.json());

        if (saved.error) {
            if (note) note.textContent = saved.error;

            return false;
        }

        if (note) note.textContent = 'Saved.';

        return true;
    } catch {
        if (note) note.textContent = 'Could not save.';

        return false;
    }
}

const form = el('settings-form');

if (form) {
    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        await save({
            name: el('set-name').value,
            owner: el('set-owner').value,
            wake_word: el('set-wake').value,
            privacy: el('set-privacy').value,
        }, el('set-note'));
    });
}

const naming = el('naming-form');

if (naming) {
    naming.addEventListener('submit', async (e) => {
        e.preventDefault();

        // The placeholders are real defaults, not hints: somebody who presses
        // Begin without typing has chosen them.
        const ok = await save({
            name: el('naming-name').value || 'PN Brain',
            owner: el('naming-owner').value || 'Petar',
        }, null);

        if (ok) {
            el('naming').hidden = true;
            location.reload();
        }
    });
}

document.querySelector('.nav-item[data-view="system"]')?.addEventListener('click', load);

load();

})();
