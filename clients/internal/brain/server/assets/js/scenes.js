/*
 * Starts the two things drawn on the graphics card, and wires what happens
 * when a memory is clicked.
 *
 * Both run inside the page. They were a separate Godot program for a while,
 * placed over the panels in a window of its own — it worked, and it was the
 * wrong shape: a second process that had to be told where to be, could drift
 * out of place, could not respect a rounded corner, and added seventy megabytes
 * to a seven megabyte program.
 */

import { startCore } from './core3d.js';
import { startMemoryMap } from './memorymap3d.js';

startCore();
startMemoryMap();

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
