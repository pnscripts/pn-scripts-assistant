/*
 * Who does which part of a job.
 *
 * Two things are changeable here — which size of model somebody uses, and
 * whether they are pinned to a particular service — because those are the two
 * anybody actually wants to change. Tool lists and standing instructions stay
 * in the files: they are written once and read often, and a form with eight
 * fields in it is a form nobody finishes.
 */
(function () {
    'use strict';

    const el = (id) => document.getElementById(id);

    const SIZES = [
        ['work', 'the working model'],
        ['quick', 'the fastest one'],
        ['best', 'the ablest one'],
        ['reason', 'one that thinks first'],
        ['talk', 'the small one']
    ];

    function say(text) {
        const node = el('team-said');
        if (node) node.textContent = text || '';
    }

    function label(text, className) {
        const span = document.createElement('span');
        span.className = className;
        span.textContent = text;
        return span;
    }

    function agentRow(agent, providers) {
        const row = document.createElement('div');
        row.className = 'agent';

        const head = document.createElement('div');
        head.className = 'agent-head';

        head.appendChild(label(agent.title || agent.name, 'agent-title'));

        // What it is for, which is also how the planner chooses it.
        head.appendChild(label(agent.for, 'agent-for'));

        if (!agent.built_in) head.appendChild(label('changed', 'skill-origin'));

        row.appendChild(head);

        const controls = document.createElement('div');
        controls.className = 'agent-controls';

        // Which size of model, and what that actually is on this machine — so
        // the choice is between real models rather than between words.
        const uses = document.createElement('select');

        for (const [value, text] of SIZES) {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = text;
            option.selected = (agent.uses || 'work') === value;
            uses.appendChild(option);
        }

        controls.appendChild(uses);

        const on = document.createElement('select');

        const anywhere = document.createElement('option');
        anywhere.value = '';
        anywhere.textContent = 'whatever the job is using';
        anywhere.selected = !agent.provider;
        on.appendChild(anywhere);

        for (const name of providers) {
            const option = document.createElement('option');
            option.value = name;
            option.textContent = 'always ' + name;
            option.selected = agent.provider === name;
            on.appendChild(option);
        }

        controls.appendChild(on);

        const save = document.createElement('button');
        save.type = 'button';
        save.className = 'task-button';
        save.textContent = 'Save';
        save.addEventListener('click', () => saveAgent(agent.name, uses.value, on.value));
        controls.appendChild(save);

        if (!agent.built_in) {
            const reset = document.createElement('button');
            reset.type = 'button';
            reset.className = 'task-button';
            reset.textContent = 'Put back';
            reset.addEventListener('click', () => resetAgent(agent.name));
            controls.appendChild(reset);
        }

        row.appendChild(controls);

        const note = document.createElement('div');
        note.className = 'agent-note';

        const tools = agent.tools || [];
        note.textContent = (agent.model || 'no model installed')
            + ' · ' + (tools.length === 1 && tools[0] === 'everything'
                ? 'every tool'
                : tools.length + ' tools: ' + tools.join(', '));

        row.appendChild(note);

        return row;
    }

    async function saveAgent(name, uses, provider) {
        say('Saving…');

        try {
            const res = await fetch('/api/team/' + encodeURIComponent(name), {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ uses: uses, provider: provider })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say(body.error || 'That did not save.');
                return;
            }

            say('Saved. It takes effect on the next step given to them.');
            load();
        } catch (err) {
            say('That did not save: ' + err.message);
        }
    }

    async function resetAgent(name) {
        say('Putting ' + name + ' back…');

        try {
            const res = await fetch('/api/team/' + encodeURIComponent(name) + '/reset', {
                method: 'POST'
            });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say(body.error || 'That did not work.');
                return;
            }

            say('Back to what it shipped as.');
            load();
        } catch (err) {
            say('That did not work: ' + err.message);
        }
    }

    async function load() {
        let body;

        try {
            const res = await fetch('/api/team');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const list = el('team-list');
        if (!list) return;

        list.textContent = '';

        for (const agent of (body.team || [])) {
            list.appendChild(agentRow(agent, body.providers || []));
        }

        const folder = el('team-folder');
        if (folder) {
            folder.textContent = 'Their tools and standing instructions are files in '
                + (body.folder || '') + '.';
        }
    }

    function wire() {
        load();

        // Read when the view is opened rather than on a timer. Nothing here
        // changes on its own, and a poll over a row of dropdowns would only
        // ever be a way to reset one somebody had just changed.
        document.querySelector('.nav-item[data-view="tasks"]')
            ?.addEventListener('click', load);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
