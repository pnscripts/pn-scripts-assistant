/*
 * Starts the three things drawn on the graphics card — the core, the memory
 * map and the stage the panels stand in — and wires what happens when a
 * memory is clicked.
 *
 * All of them run inside the page. They were a separate Godot program for a while,
 * placed over the panels in a window of its own — it worked, and it was the
 * wrong shape: a second process that had to be told where to be, could drift
 * out of place, could not respect a rounded corner, and added seventy megabytes
 * to a seven megabyte program.
 */

import { startCore } from './core3d.js';
import { startMemoryMap } from './memorymap3d.js';
import { startStage } from '/stage/stage.js';
import { signals } from './signals.js';

/*
 * Each on its own, so one that cannot start does not take the others with it.
 *
 * All three are modules in one file, and an exception thrown by the first
 * stopped the file: a core that could not get a context left the page without
 * a memory map and without the stage, which is to say without its way round
 * once the column of places is hidden.
 */
for (const start of [startCore, startMemoryMap]) {
    try {
        start();
    } catch (err) {
        console.warn('A scene could not start.', err);
    }
}

/* ---------- the stage ---------- */

/*
 * Every view stands in the world, in the order the column lists them.
 *
 * So the panel beside this one on the ring is the entry beside this one in the
 * column, and turning to it is a short turn. The memory map has an entry and
 * no view of its own — it grows inside the command centre — so it takes no
 * place on the ring.
 */
const entryOf = new Map();

for (const item of document.querySelectorAll('.nav-item[data-view]')) {
    const view = document.querySelector(`section.view[data-view="${item.dataset.view}"]`);

    if (view && !entryOf.has(view)) entryOf.set(view, item);
}

function statusColour() {
    const status = document.documentElement.dataset.status || 'idle';

    return getComputedStyle(document.documentElement).getPropertyValue(`--status-${status}`).trim();
}

startStage({
    frame: document.querySelector('.main'),
    panels: [...entryOf.keys()],
    fill: true,
    title: (view) => entryOf.get(view)?.querySelector('span')?.textContent.trim() || view.dataset.view,
    count: (view) => {
        const badge = entryOf.get(view)?.querySelector('.nav-count');

        return badge && !badge.hidden && badge.textContent.trim() !== '0' ? badge.textContent.trim() : '';
    },
    icon: (view) => entryOf.get(view)?.querySelector('svg'),
    go: (view) => entryOf.get(view)?.click(),
    // The light in the middle is the brain's status colour and the measured
    // sound level: the same two readings the core draws, from the same place.
    pulse: () => ({ colour: statusColour(), level: signals.level }),
    button: document.getElementById('stage-overview'),
    turner: {
        root: document.getElementById('stage-turner'),
        prev: document.getElementById('stage-prev'),
        here: document.getElementById('stage-here'),
        next: document.getElementById('stage-next'),
    },
});

/*
 * Everything waiting, on the button that shows every panel.
 *
 * The column put a number beside each place with something in it — updates to
 * install, tasks that need an answer. With the column out of sight those
 * numbers would be kept and never seen, so they are added up here, and each
 * panel out in the world still carries its own.
 */
const waiting = document.getElementById('stage-waiting');
const counts = [...document.querySelectorAll('.nav-item .nav-count')];

function addUpWaiting() {
    if (!waiting || !document.documentElement.classList.contains('has-stage')) return;

    const total = counts
        .filter((badge) => !badge.hidden)
        .reduce((sum, badge) => sum + (parseInt(badge.textContent, 10) || 0), 0);

    waiting.textContent = String(total);
    waiting.hidden = total === 0;
}

const recount = new MutationObserver(addUpWaiting);

for (const badge of counts) {
    recount.observe(badge, { attributes: true, attributeFilter: ['hidden'], childList: true, characterData: true, subtree: true });
}

addUpWaiting();

/* ---------- clicking a memory ---------- */

const detail = document.getElementById('memory-detail');
const hint = document.getElementById('memory-hover');

function el(id) {
    return document.getElementById(id);
}

document.addEventListener('memory-selected', (e) => {
    if (!detail) return;

    const memory = e.detail;

    detail.hidden = false;
    el('memory-detail-category').textContent = memory.category || 'memory';
    el('memory-detail-text').textContent = memory.label || '';

    const related = el('memory-detail-related');

    related.innerHTML = '';

    if (!memory.related.length) {
        const none = document.createElement('p');

        none.className = 'note';
        none.textContent = 'Nothing else it considers close to this.';
        related.append(none);
    }

    for (const near of memory.related.slice(0, 8)) {
        const row = document.createElement('button');

        row.type = 'button';
        row.className = 'related-item';
        row.textContent = near.label;
        related.append(row);
    }

    // Asking about it puts the memory in the message box rather than sending
    // anything: what to do with it is the reader's decision, not the map's.
    el('memory-ask').onclick = () => {
        const input = el('input');

        if (!input) return;

        input.value = `Tell me what you know about: ${memory.label}`;
        input.focus();
    };
});

document.addEventListener('memory-cleared', () => {
    if (detail) detail.hidden = true;
});

document.addEventListener('memory-hovered', (e) => {
    if (!hint) return;

    hint.textContent = e.detail ? e.detail.label : '';
    hint.hidden = !e.detail;
});

const close = document.getElementById('memory-detail-close');

if (close) {
    close.addEventListener('click', () => {
        if (detail) detail.hidden = true;
    });
}
