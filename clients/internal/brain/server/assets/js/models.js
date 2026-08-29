/*
 * Which model the brain runs on, and whether it is any good at this machine.
 *
 * Choosing one is a real decision and the numbers that decide it are local. A
 * model that is quick on a graphics card can be unusable on four processor
 * cores, and a larger model is not reliably a better one: measured here, the
 * 3B answered in a third of the time of the 8B and was the only one of the two
 * larger models that asked for its tools properly rather than writing the call
 * out as text.
 *
 * So the test is run here, against the machine it will actually run on, and the
 * result is shown rather than a claim about the model.
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


const list = document.getElementById('models-list');
const note = document.getElementById('models-note');

let inUse = '';
const measured = new Map();

async function load() {
    if (!list) return;

    try {
        const data = await fetch('/api/models').then((r) => r.json());

        if (data.error) {
            note.textContent = data.error;
            list.innerHTML = '';

            return;
        }

        inUse = data.chat || '';
        render(data);
    } catch {
        note.textContent = 'Ollama is not answering, so there is nothing to choose from.';
    }
}

function render(data) {
    const usable = (data.installed || []).filter((m) => !/embed|bge/i.test(m.name));

    note.textContent = usable.length
        ? 'Test one to see how long it takes here and whether it uses tools properly. '
            + 'The memory model is not listed: every memory was embedded with it, '
            + 'and changing it would make recall meaningless rather than different.'
        : 'Nothing is installed that can hold a conversation.';

    list.innerHTML = '';

    for (const model of usable) {
        const row = document.createElement('div');

        row.className = 'row' + (model.name === inUse ? ' ok' : '');

        const icon = document.createElement('span');
        icon.className = 'row-icon';
        icon.textContent = model.size.replace(/[^0-9.]/g, '').slice(0, 3);

        const text = document.createElement('span');
        text.className = 'row-text';

        const name = document.createElement('span');
        name.className = 'row-name';
        name.textContent = model.name + (model.name === inUse ? '  · in use' : '');

        const state = document.createElement('span');
        state.className = 'row-state';

        const result = measured.get(model.name);

        state.textContent = result
            ? `${result.seconds.toFixed(0)}s · ${result.note}`
            : `${model.size} · not tested here yet`;

        text.append(name, state);

        const test = document.createElement('button');
        test.type = 'button';
        test.className = 'model-action';
        test.textContent = 'Test';
        test.onclick = () => measure(model.name, test);

        const use = document.createElement('button');
        use.type = 'button';
        use.className = 'model-action';
        use.textContent = 'Use';
        use.disabled = model.name === inUse;
        use.onclick = () => choose(model.name);

        row.append(icon, text, test, use);
        list.append(row);
    }
}

async function measure(name, button) {
    button.disabled = true;
    button.textContent = 'Testing…';

    try {
        const result = await fetch('/api/models/measure', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name }),
        }).then((r) => r.json());

        if (!result.error) measured.set(name, result);
    } catch {
        /* The row simply stays untested. */
    }

    load();
}

async function choose(name) {
    try {
        await fetch('/api/models/use', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name }),
        });
    } catch {
        /* Nothing changes if it failed. */
    }

    load();
}

document.querySelector('.nav-item[data-view="models"]')?.addEventListener('click', load);

load();

})();
