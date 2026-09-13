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

/*
 * Every status, in the order it happens during a turn.
 *
 * All of them, not the four the core used to know. The panel offering fewer
 * colours than the interface actually shows is how three of them ended up
 * unchangeable — and the ones left out were the ones somebody is most likely
 * to want moved, since "working out what you said" and "thinking" sit next to
 * each other in meaning and have to be far apart in colour.
 *
 * The wording under each is when it appears, because a colour picker beside
 * the word "hearing" tells nobody which moment it paints.
 */
const COLOUR_PARTS = [
    ['idle', 'At rest', 'Nothing happening'],
    ['listening', 'Listening', 'While the microphone is open'],
    ['hearing', 'Hearing you', 'While it works out what you said'],
    ['thinking_core', 'Thinking', 'While a model is working'],
    ['tool', 'Doing something', 'While it reads, searches or opens something'],
    ['speaking', 'Its own voice', 'While it is speaking'],
    ['waiting', 'Needs you', 'When it has stopped and wants an answer'],
    ['learning', 'Learning', 'Its own background work, nobody waiting'],
    ['thinking_line', 'The sweeping line', 'The band that crosses the core'],
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

    /*
     * Which model answers what, and whether switching between them is free.
     *
     * The failure this shows is otherwise invisible and reads as the program
     * being slow: a switch evicts the other model, the next turn reloads
     * several gigabytes, and nothing says why.
     */
    if (el('models-roles') && status.models) {
        const m = status.models;

        const jobs = [
            ['talking', m.talk],
            ['doing things', m.work],
            ['working things out', m.reason],
        ].filter(([, model]) => model);

        el('models-roles').innerHTML = jobs
            .map(([job, model]) => `<span class="model-job">${job}</span> ${model}`)
            .join('<br>');

        const warning = el('models-warning');

        if (warning) {
            warning.hidden = !m.talk || m.talk === m.work || m.both_resident;

            el('models-fix').textContent =
                'sudo mkdir -p /etc/systemd/system/ollama.service.d && '
                + "printf '[Service]\\nEnvironment=\"OLLAMA_MAX_LOADED_MODELS=3\"\\n' "
                + '| sudo tee /etc/systemd/system/ollama.service.d/models.conf && '
                + 'sudo systemctl daemon-reload && sudo systemctl restart ollama';
        }
    }

    /*
     * The form is refilled from the server, but never while it is being filled
     * in by a person.
     *
     * This panel is refreshed every three seconds, and refreshing meant
     * overwriting every field with what the server currently holds. So
     * choosing something from the privacy dropdown and taking more than three
     * seconds to reach the Save button put the old value back underneath the
     * cursor — the setting appeared not to work, and what was actually
     * happening was that it was being un-set faster than it could be saved.
     *
     * Nothing is written back once somebody has touched the form, until they
     * save it. A stale field they are editing is not stale: it is theirs.
     */
    if (el('set-name') && !beingEdited()) {
        el('set-name').value = status.name || '';
        el('set-owner').value = status.owner || '';
        el('set-wake').value = status.wake_word || '';
        el('set-privacy').value = (status.privacy && status.privacy.mode) || 'private';
        el('set-always').checked = status.always_name !== false;
        el('set-auto-model').checked = status.auto_model !== false;

        // Defaults to on: an assistant you can talk to should answer out loud
        // unless somebody has said otherwise.
        el('set-always-listen').checked = status.always_listen !== false;
        el('set-always-speak').checked = status.always_speak !== false;
    }

    renderColours();

    /*
     * And the page repaints without a reload.
     *
     * The palette is read into CSS variables at load; changing a colour and
     * seeing nothing move until a restart is the kind of thing that makes
     * somebody change it twice more, wondering whether it worked.
     */
    if (window.brainRepaintStatuses) window.brainRepaintStatuses();
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
    hearing: 'hearing',
    thinking_core: 'core',
    tool: 'tool',
    speaking: 'speaking',
    waiting: 'waiting',
    learning: 'learning',
    thinking_line: 'thinking line',
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

/*
 * beingEdited reports that the settings form is somebody's to change.
 *
 * True while any of its fields has focus, and true from the first keystroke or
 * choice until it is saved — because a change made and then clicked away from
 * is still a change waiting to be saved, and putting the old value back under
 * it would be the same fault a moment later.
 */
let touched = false;

function beingEdited() {
    const form = el('settings-form');

    if (!form) return false;

    return touched || form.contains(document.activeElement);
}

const settingsForm = el('settings-form');

if (settingsForm) {
    for (const event of ['input', 'change']) {
        settingsForm.addEventListener(event, () => { touched = true; });
    }
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

        // Saved, so the server's answer is the truth again and the form may be
        // refilled from it.
        touched = false;

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
            always_name: el('set-always').checked,
            auto_model: el('set-auto-model').checked,
            always_listen: el('set-always-listen').checked,
            always_speak: el('set-always-speak').checked,
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
            name: el('naming-name').value || 'Assistant',
            owner: el('naming-owner').value || 'Petar',
        }, null);

        if (ok) {
            el('naming').hidden = true;
            location.reload();
        }
    });
}


/*
 * What the microphone made of the room, and what became of it.
 *
 * This panel exists because of a specific afternoon: the brain stopped
 * answering to its name and there was no way, from inside the program, to find
 * out why. Four different faults look identical from a chair — the room never
 * crossed the threshold, it crossed and no words came back, words came back
 * without the name in them, or the name was there under a spelling nobody
 * expected. The transcript and three numbers tell them apart immediately.
 *
 * It was the fourth: a microphone that writes "PN Brain" down as "Piembring".
 * Which is why each line offers to add what it heard to the list of names —
 * the fix for that is a word, and it should not require knowing where the
 * settings file lives.
 */
/*
 * Opening setup from a brain that is already running.
 *
 * The wizard used to appear only when the machine was missing something, so
 * every choice it asks about — the drive, the model, the keys — could be made
 * once and never again without a terminal. The promise is that everything
 * works from inside the program, and "everything" has to include changing your
 * mind.
 */
function wireSetup() {
    const open = el('setup-open');
    const note = el('setup-note');

    if (!open) return;

    open.onclick = async () => {
        open.disabled = true;

        if (note) note.textContent = 'Opening setup…';

        try {
            await api.post('/api/setup', {});

            if (note) note.textContent = 'Setup is open in its own window.';
        } catch (err) {
            // Named, not swallowed: a button that does nothing and says nothing
            // is indistinguishable from a broken program.
            if (note) note.textContent = 'Could not open setup: ' + err.message;
        }

        open.disabled = false;
    };
}

wireSetup();

/*
 * Being in the applications menu.
 *
 * A single file that runs when double-clicked is the right shape for this
 * program and the wrong shape for launching it a second time: nobody goes
 * looking for the file again, they press the Ubuntu key and type a few
 * letters. Offered here rather than only as a command, because the promise is
 * that everything works from inside the program.
 */
async function loadMenu() {
    const note = el('menu-note');
    const add = el('menu-add');
    const remove = el('menu-remove');

    if (!note || !add || !remove) return;

    let status;

    try {
        status = await api.get('/api/desktop');
    } catch {
        return;
    }

    note.textContent = status.installed
        ? `In the menu. Press the Ubuntu key and type its name. \u00b7 ${status.entry}`
        : 'Not in the menu yet. Adding it writes one file and a set of icons '
            + 'into your own home directory — nothing else on the machine changes.';

    add.hidden = status.installed;
    remove.hidden = !status.installed;
}

function wireMenu() {
    const press = async (button, body, working) => {
        if (!button) return;

        button.addEventListener('click', async () => {
            const was = button.textContent;

            button.disabled = true;
            button.textContent = working;

            try {
                await api.post('/api/desktop', body);
                await loadMenu();
            } catch (err) {
                el('menu-note').textContent = String(err.message || err);
            } finally {
                button.disabled = false;
                button.textContent = was;
            }
        });
    };

    press(el('menu-add'), {}, 'Adding\u2026');
    press(el('menu-remove'), { remove: true }, 'Removing\u2026');
}

wireMenu();

// A woman's voice or a man's, which is the question people actually have.
document.querySelectorAll('.voice-choice [data-sex]').forEach((button) => {
    button.addEventListener('click', async () => {
        const note = el('set-note');

        button.disabled = true;

        try {
            const chosen = await api.post('/api/voice/sex', { sex: button.dataset.sex });

            if (note) note.textContent = `Now using ${chosen.voice}.`;
        } catch (err) {
            if (note) note.textContent = String(err.message || err);
        } finally {
            button.disabled = false;
        }
    });
});

/*
 * The mailbox.
 *
 * The password field is never filled in from the server, and a blank one means
 * "keep what is saved" rather than "clear it". A page that echoed the password
 * back would put it in the page source, in the accessibility tree, and in any
 * screenshot of this window.
 */
async function loadMail() {
    if (!el('mail-user')) return;

    let status;

    try {
        status = await api.get('/api/mail');
    } catch {
        return;
    }

    el('mail-user').value = status.user || '';
    el('mail-host').value = status.host || '';
    el('mail-smtp').value = status.smtp || '';
    el('mail-password').placeholder = status.has_password
        ? 'saved \u00b7 leave blank to keep it'
        : 'app password';

    el('mail-note').textContent = status.configured
        ? 'Ready. Ask it to check your email.'
        : 'Not set up yet.';
}

const mailForm = el('mail-form');

if (mailForm) {
    mailForm.addEventListener('submit', async (e) => {
        e.preventDefault();

        const note = el('mail-note');

        note.textContent = 'Saving\u2026';

        try {
            await api.post('/api/mail', {
                user: el('mail-user').value.trim(),
                password: el('mail-password').value,
                host: el('mail-host').value.trim(),
                smtp: el('mail-smtp').value.trim(),
            });

            el('mail-password').value = '';
            await loadMail();
        } catch (err) {
            note.textContent = String(err.message || err);
        }
    });
}

async function loadHeard() {
    const rows = el('heard-rows');

    if (!rows) return;

    let data;

    try {
        data = await api.get('/api/heard');
    } catch {
        return;
    }

    rows.innerHTML = '';

    const note = el('heard-note');

    if (!data.turns || !data.turns.length) {
        if (note) {
            note.textContent = data.wake_word
                ? `Nothing heard yet. It is listening for \u201c${data.wake_word}\u201d.`
                : 'Nothing heard yet.';
        }

        return;
    }

    if (note) {
        note.textContent = `It answers to: ${(data.names || []).join(', ') || 'anything'}`;
    }

    for (const turn of data.turns) {
        const row = document.createElement('div');
        row.className = `row heard-row${turn.addressed ? ' is-addressed' : ''}`;

        const when = new Date(turn.at).toLocaleTimeString();
        const said = (turn.text || '').trim();

        const what = document.createElement('div');
        what.className = 'heard-what';
        what.textContent = said ? `\u201c${said}\u201d` : '(no words)';

        const why = document.createElement('div');
        why.className = 'heard-why';
        why.textContent = `${when} \u00b7 ${turn.why} \u00b7 loudest ${turn.peak_rms}, `
            + `room ${turn.noise_floor}, needed ${turn.threshold}`;

        const text = document.createElement('div');
        text.appendChild(what);
        text.appendChild(why);
        row.appendChild(text);

        /*
         * One word is the whole fix, when the fix is a mishearing.
         *
         * Offered only for a line that has words in it and was not acted on:
         * adding a name it already answers to would do nothing, and adding
         * silence would break it.
         */
        if (said && !turn.addressed) {
            const first = said.split(/[\s,.!?]+/)[0];

            if (first) {
                const add = document.createElement('button');
                add.type = 'button';
                add.className = 'model-action';
                add.textContent = `Also answer to \u201c${first}\u201d`;
                add.addEventListener('click', async () => {
                    add.disabled = true;

                    const now = el('set-wake').value.trim();

                    try {
                        await api.post('/api/settings', {
                            wake_word: now ? `${now}, ${first}` : first,
                        });
                        await load();
                        await loadHeard();
                    } catch (err) {
                        add.disabled = false;
                        add.textContent = String(err.message || err);
                    }
                });
                row.appendChild(add);
            }
        }

        rows.appendChild(row);
    }
}

document.querySelector('.nav-item[data-view="system"]')?.addEventListener('click', () => {
    load();
    loadHeard();
    loadMenu();
    loadMail();
});

/*
 * Refreshed while the panel is open.
 *
 * All of it, not only what the microphone heard. A window left open across a
 * restart kept showing what was true when the page loaded — which is how the
 * models card came to read "—" long after the brain had chosen three of them.
 * A panel that is wrong and still is a worse failure than one that is empty,
 * because there is nothing about it that looks stale.
 */
setInterval(() => {
    const view = document.querySelector('.view[data-view="system"]');

    if (!view || view.hidden) return;

    loadHeard();
    load();
    loadMenu();
}, 3000);

load();
loadHeard();
loadMenu();
loadMail();

})();
