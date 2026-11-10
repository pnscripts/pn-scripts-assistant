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
/*
 * Wrapped, because the plain scripts share one global scope.
 *
 * Names like list, note and render are the obvious ones to reach for in a file
 * that fills a panel, and two files reaching for the same one is a SyntaxError
 * — which does not fail the line, it fails the whole file. This one was dead
 * for that reason, and nothing on screen said so: the panel simply stayed
 * empty, which looks like having no data rather than like a broken script.
 */
(function () {


const list = document.getElementById('updates-list');
const note = document.getElementById('updates-note');

// Every word a person reads here is in i18n.js, in English and Bulgarian.
const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
const bg = () => !!(window.pnLang && window.pnLang() === 'bg');

// A string named by a key the server gives, or the English it came with.
function said(key, fallback, vars) {
    const text = T(key, vars);

    return text === key ? fallback : text;
}

if (note) note.textContent = T('mdl.looking');

const TONE = {
    running: 'ok',
    installed: 'ok',
    answering: 'ok',
    remembering: 'ok',
    configured: 'warn',
    not_found: 'warn',
    not_configured: 'off',
    unknown: 'off',
};

async function load() {
    if (!list) return;

    try {
        const data = await fetch('/api/updates').then((r) => r.json());

        render(data.parts || []);
    } catch {
        note.textContent = T('parts.unreadable');
    }
}

function render(parts) {
    const paid = parts.filter((p) => p.cost && p.cost !== 'free' && p.cost !== '—');

    note.textContent = paid.length ? T('parts.freeExcept') : T('parts.allFree');

    list.innerHTML = '';

    for (const part of parts) {
        const row = document.createElement('div');

        row.className = `row ${TONE[String(part.status).replace(/\W+/g, '_')] || ''}`;

        const icon = document.createElement('span');
        icon.className = 'row-icon';
        icon.textContent = part.cost === 'free' ? T('parts.free') : part.cost === '—' ? '—' : T('parts.paid');

        const text = document.createElement('span');
        text.className = 'row-text';

        const name = document.createElement('span');
        name.className = 'row-name';
        const called = part.name_key ? said(`parts.called.${part.name_key}`, part.name) : part.name;
        // A model's "version" is its size, written with a decimal comma in
        // Bulgarian; a real version number keeps its points.
        const sized = /^\d+(\.\d+)? ?[KMGT]?B$/.test(String(part.version || ''));
        const version = bg() && sized ? part.version.replace('.', ',') : part.version;

        name.textContent = version ? `${called} · ${version}` : called;

        const state = document.createElement('span');
        state.className = 'row-state';
        const status = said(`parts.status.${String(part.status).replace(/\W+/g, '_')}`, part.status);
        const told = part.note_key ? said(`parts.note.${part.note_key}`, part.note, part.args || {}) : part.note;

        state.textContent = `${status}: ${told}`;

        text.append(name, state);
        row.append(icon, text);
        list.append(row);
    }
}

document.querySelector('.nav-item[data-view="updates"]')?.addEventListener('click', load);

// Drawn again in the other language as soon as it changes.
window.addEventListener('pn-language', load);

load();

})();
