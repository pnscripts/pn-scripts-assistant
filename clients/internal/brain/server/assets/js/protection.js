/*
 * What the brain asks about before it reads.
 *
 * This panel used to have no equivalent because there was nothing to decide:
 * the list was a refusal, hardcoded, and its owner could not see it or change
 * it. That was the wrong shape for a program running on somebody's own machine
 * against their own files — a thing that answers "no" to its owner has decided
 * something that was not its to decide.
 *
 * So it asks instead, and this is where the asking is set: which categories
 * are worth a question, anything of your own to add, and — the half that grows
 * by itself — what it has learned it may read, each of which can be taken
 * back.
 */
(function () {

function el(id) {
    return document.getElementById(id);
}

function say(what, kind) {
    const note = el('protect-note');

    if (!note) return;

    note.textContent = what || '';
    note.dataset.kind = kind || '';
}

let current = { rules: [], yours: [], learned: [] };

async function send(body) {
    const res = await fetch('/api/protection', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
    });

    const data = await res.json().catch(() => ({}));

    if (!res.ok) throw new Error(data.error || `that did not work (${res.status})`);

    return data;
}

async function load() {
    try {
        current = await fetch('/api/protection').then((r) => r.json());
    } catch {
        return;
    }

    draw();
}

function draw() {
    drawRules();
    drawYours();
    drawLearned();
}

function drawRules() {
    const box = el('protect-rules');

    if (!box) return;

    box.innerHTML = '';

    (current.rules || []).forEach((rule) => {
        const row = document.createElement('label');
        row.className = 'field-check protect-rule';

        const tick = document.createElement('input');
        tick.type = 'checkbox';
        tick.checked = rule.on;
        tick.disabled = rule.fixed;

        /*
         * The reason next to the choice, not behind a help icon.
         *
         * "/.aws/" is not something anybody can make a decision about. What it
         * is and what is in it are the decision.
         */
        const text = document.createElement('span');
        text.innerHTML = `<b>${rule.what}</b>${rule.fixed
            ? ' <span class="protect-fixed">always</span>' : ''}<br><span class="note">${rule.why}</span>`;

        tick.onchange = async () => {
            const off = (current.rules || [])
                .filter((r) => (r.id === rule.id ? !tick.checked : !r.on))
                .filter((r) => !r.fixed)
                .map((r) => r.id);

            try {
                current = await send({ off });
                say(tick.checked
                    ? `It will ask about ${rule.what.toLowerCase()} again.`
                    : `It will no longer ask about ${rule.what.toLowerCase()}.`, 'ok');
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            draw();
        };

        row.append(tick, text);
        box.appendChild(row);
    });
}

function drawYours() {
    const box = el('protect-yours');

    if (!box) return;

    box.innerHTML = (current.yours || []).map((own) => `<div class="row">
        <span class="row-label">${own}</span>
        <button type="button" class="model-action" data-drop="${own.replace(/"/g, '&quot;')}">Remove</button>
    </div>`).join('');

    box.querySelectorAll('[data-drop]').forEach((button) => {
        button.onclick = async () => {
            try {
                current = await send({
                    yours: (current.yours || []).filter((y) => y !== button.dataset.drop),
                });

                say('Removed.', 'ok');
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            draw();
        };
    });
}

function drawLearned() {
    const card = el('protect-learned-card');
    const box = el('protect-learned');

    if (!card || !box) return;

    const learned = current.learned || [];

    card.hidden = learned.length === 0;

    box.innerHTML = learned.map((path) => `<div class="row">
        <span class="row-label" title="${path.replace(/"/g, '&quot;')}">${path}</span>
        <button type="button" class="model-action" data-forget="${path.replace(/"/g, '&quot;')}">Ask me again</button>
    </div>`).join('');

    box.querySelectorAll('[data-forget]').forEach((button) => {
        button.onclick = async () => {
            try {
                current = await send({
                    learned: learned.filter((p) => p !== button.dataset.forget),
                });
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            draw();
        };
    });
}

const add = el('protect-save');

if (add) {
    add.onclick = async () => {
        const what = (el('protect-add').value || '').trim();

        if (!what) {
            say('Type a folder or a file name.', 'bad');

            return;
        }

        try {
            current = await send({ yours: [...(current.yours || []), what] });
            el('protect-add').value = '';
            say(`It will ask before opening anything matching ${what}.`, 'ok');
        } catch (err) {
            say(String(err.message || err), 'bad');
        }

        draw();
    };
}

load();

}());
