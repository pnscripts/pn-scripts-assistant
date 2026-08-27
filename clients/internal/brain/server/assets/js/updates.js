/*
 * What the brain is made of, and what any of it costs.
 *
 * The cost column is the reason this exists. Everything the brain runs on is on
 * this machine and free to use, and saying so plainly is worth more than
 * leaving somebody to wonder whether a meter is running. Where a paid service
 * is configured it says so and says it is billed by whoever provides it — no
 * rate is printed, because a rate copied into a program goes out of date
 * without anyone noticing, and a wrong price is worse than none.
 */

const list = document.getElementById('updates-list');
const note = document.getElementById('updates-note');

const TONE = {
    running: 'ok',
    installed: 'ok',
    answering: 'ok',
    remembering: 'ok',
    configured: 'warn',
    'not found': 'warn',
    'not configured': 'off',
    unknown: 'off',
};

async function load() {
    if (!list) return;

    try {
        const data = await fetch('/api/updates').then((r) => r.json());

        render(data.parts || []);
    } catch {
        note.textContent = 'Could not read what is installed.';
    }
}

function render(parts) {
    const paid = parts.filter((p) => p.cost && p.cost !== 'free' && p.cost !== '—');

    note.textContent = paid.length
        ? 'Everything below runs on this machine and is free, except where marked.'
        : 'Everything below runs on this machine. Nothing here costs anything to use, '
            + 'and nothing is sent anywhere.';

    list.innerHTML = '';

    for (const part of parts) {
        const row = document.createElement('div');

        row.className = `row ${TONE[part.status] || ''}`;

        const icon = document.createElement('span');
        icon.className = 'row-icon';
        icon.textContent = part.cost === 'free' ? 'free' : part.cost === '—' ? '—' : 'paid';

        const text = document.createElement('span');
        text.className = 'row-text';

        const name = document.createElement('span');
        name.className = 'row-name';
        name.textContent = part.version ? `${part.name} · ${part.version}` : part.name;

        const state = document.createElement('span');
        state.className = 'row-state';
        state.textContent = `${part.status} — ${part.note}`;

        text.append(name, state);
        row.append(icon, text);
        list.append(row);
    }
}

document.querySelector('.nav-item[data-view="updates"]')?.addEventListener('click', load);

load();
