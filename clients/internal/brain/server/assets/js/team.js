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
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    // The sizes of model somebody can be given; their words are task.size.*.
    const SIZES = ['work', 'quick', 'best', 'reason', 'talk'];

    // A key's words when it has them, the given text when it does not.
    function maybe(key, otherwise) {
        const said = T(key);

        return said === key ? otherwise : said;
    }

    // The name and purpose of a built-in specialist in the page's language;
    // one somebody changed says what they wrote.
    function titleOf(agent) {
        const own = agent.title || agent.name;

        return agent.built_in ? maybe('task.agent.' + agent.name, own) : own;
    }

    function forOf(agent) {
        return agent.built_in ? maybe('task.agentFor.' + agent.name, agent.for) : agent.for;
    }

    function toolName(name) {
        return window.pnToolName ? window.pnToolName(name) : name;
    }

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

        head.appendChild(label(titleOf(agent), 'agent-title'));

        // What it is for, which is also how the planner chooses it.
        head.appendChild(label(forOf(agent), 'agent-for'));

        if (!agent.built_in) head.appendChild(label(T('task.changed'), 'skill-origin'));

        row.appendChild(head);

        const controls = document.createElement('div');
        controls.className = 'agent-controls';

        // Which size of model, and what that actually is on this machine — so
        // the choice is between real models rather than between words.
        const uses = document.createElement('select');

        for (const value of SIZES) {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = T(`task.size.${value}`);
            option.selected = (agent.uses || 'work') === value;
            uses.appendChild(option);
        }

        controls.appendChild(uses);

        const on = document.createElement('select');

        const anywhere = document.createElement('option');
        anywhere.value = '';
        anywhere.textContent = T('task.anyProvider');
        anywhere.selected = !agent.provider;
        on.appendChild(anywhere);

        for (const name of providers) {
            const option = document.createElement('option');
            option.value = name;
            option.textContent = T('task.always', { name: name });
            option.selected = agent.provider === name;
            on.appendChild(option);
        }

        controls.appendChild(on);

        const save = document.createElement('button');
        save.type = 'button';
        save.className = 'task-button';
        save.textContent = T('task.save');
        save.addEventListener('click', () => saveAgent(agent.name, uses.value, on.value));
        controls.appendChild(save);

        if (!agent.built_in) {
            const reset = document.createElement('button');
            reset.type = 'button';
            reset.className = 'task-button';
            reset.textContent = T('task.putBack');
            reset.addEventListener('click', () => resetAgent(agent.name));
            controls.appendChild(reset);
        }

        row.appendChild(controls);

        const note = document.createElement('div');
        note.className = 'agent-note';

        const tools = agent.tools || [];
        note.textContent = (agent.model || T('task.noModel'))
            + ' · ' + (tools.length === 1 && tools[0] === 'everything'
                ? T('task.everyTool')
                : T('task.tools', { n: tools.length, list: tools.map(toolName).join(', ') }));

        row.appendChild(note);

        return row;
    }

    async function saveAgent(name, uses, provider) {
        say(T('task.saving'));

        try {
            const res = await fetch('/api/team/' + encodeURIComponent(name), {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ uses: uses, provider: provider })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say(body.error ? T('task.notSavedWhy', { why: body.error }) : T('task.notSaved'));
                return;
            }

            say(T('task.savedAgent'));
            load();
        } catch (err) {
            say(T('task.notSavedWhy', { why: err.message }));
        }
    }

    async function resetAgent(name) {
        say(T('task.puttingBack', { name: name }));

        try {
            const res = await fetch('/api/team/' + encodeURIComponent(name) + '/reset', {
                method: 'POST'
            });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say(body.error ? T('task.failedWhy', { why: body.error }) : T('task.failed'));
                return;
            }

            say(T('task.putBackDone'));
            load();
        } catch (err) {
            say(T('task.failedWhy', { why: err.message }));
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

        // The English titles, so a sentence the program wrote about one
        // ("waiting on the developer") can name it in the page's language.
        const byTitle = {};
        for (const agent of (body.team || [])) {
            if (agent.title) byTitle[agent.title.toLowerCase()] = agent.name;
        }
        window.pnTeamByTitle = byTitle;

        for (const agent of (body.team || [])) {
            list.appendChild(agentRow(agent, body.providers || []));
        }

        const folder = el('team-folder');
        if (folder) {
            folder.textContent = T('task.teamFolder', { folder: body.folder || '' });
        }
    }

    function wire() {
        load();

        // Read when the view is opened rather than on a timer. Nothing here
        // changes on its own, and a poll over a row of dropdowns would only
        // ever be a way to reset one somebody had just changed.
        document.querySelector('.nav-item[data-view="tasks"]')
            ?.addEventListener('click', load);

        window.addEventListener('pn-language', load);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
