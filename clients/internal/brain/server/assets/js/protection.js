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

const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

// A built-in rule's name and reason in the page's language; the server's
// English when this page has no words for a rule a later version added.
function ruleWords(rule) {
    const named = `prv.rule.${rule.id}`;
    const what = T(named);
    const why = T(`${named}.why`);

    return {
        what: what === named ? rule.what : what,
        why: why === `${named}.why` ? rule.why : why,
    };
}

// A row of one path and the button that changes it, written as text.
function pathRow(path, label, data, titled) {
    const row = document.createElement('div');
    row.className = 'row';

    const name = document.createElement('span');
    name.className = 'row-label';
    name.textContent = path;
    if (titled) name.title = path;

    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'model-action';
    button.dataset[data] = path;
    button.textContent = label;

    row.append(name, button);

    return row;
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

    if (!res.ok) throw new Error(data.error || T('prv.failed', { status: res.status }));

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
        const words = ruleWords(rule);
        const text = document.createElement('span');
        const name = document.createElement('b');
        name.textContent = words.what;
        text.appendChild(name);

        if (rule.fixed) {
            const always = document.createElement('span');
            always.className = 'protect-fixed';
            always.textContent = T('prv.always');
            text.append(' ', always);
        }

        const why = document.createElement('span');
        why.className = 'note';
        why.textContent = words.why;
        text.append(document.createElement('br'), why);

        tick.onchange = async () => {
            const off = (current.rules || [])
                .filter((r) => (r.id === rule.id ? !tick.checked : !r.on))
                .filter((r) => !r.fixed)
                .map((r) => r.id);

            try {
                current = await send({ off });
                // English says the name inside the sentence; Bulgarian quotes it.
                const named = window.pnLang && window.pnLang() === 'bg' ? words.what : words.what.toLowerCase();

                say(T(tick.checked ? 'prv.askAgain' : 'prv.noLonger', { what: named }), 'ok');
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

    box.textContent = '';
    (current.yours || []).forEach((own) => box.appendChild(pathRow(own, T('prv.remove'), 'drop', false)));

    box.querySelectorAll('[data-drop]').forEach((button) => {
        button.onclick = async () => {
            try {
                current = await send({
                    yours: (current.yours || []).filter((y) => y !== button.dataset.drop),
                });

                say(T('prv.removed'), 'ok');
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

    box.textContent = '';
    learned.forEach((path) => box.appendChild(pathRow(path, T('prv.askMeAgain'), 'forget', true)));

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
            say(T('prv.typeSomething'), 'bad');

            return;
        }

        try {
            current = await send({ yours: [...(current.yours || []), what] });
            el('protect-add').value = '';
            say(T('prv.willAsk', { what }), 'ok');
        } catch (err) {
            say(String(err.message || err), 'bad');
        }

        draw();
    };
}

load();

// Drawn here, so drawn again in the other language.
window.addEventListener('pn-language', draw);

}());
