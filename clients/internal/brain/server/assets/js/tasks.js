/*
 * Work that outlives the sentence that asked for it.
 *
 * Everything here reads the tasks rows rather than the progress endpoint. That
 * matters: progress holds one current step for the whole program and is gone
 * the moment it changes, so a task polled that way would be blank after a
 * reload, blank after a restart, and blank for the whole time it is parked
 * waiting for a decision — which is exactly when somebody is looking at it.
 */
(function () {
    'use strict';

    const el = (id) => document.getElementById(id);

    // What the page is showing in detail, kept across the poll so opening a
    // task does not close itself two seconds later.
    let open = 0;

    // Whether somebody is typing a job. The poll rebuilds these cards, and a
    // rebuild under a cursor throws away half a sentence.
    let typing = false;

    function say(text) {
        const said = el('task-said');
        if (said) said.textContent = text || '';
    }

    // A state in the owner's terms. "blocked" is a word about the program;
    // "it stopped" is a word about what happened.
    function stateOf(task) {
        switch (task.state) {
            case 'planning': return 'working out what to do';
            case 'working': return 'working';
            case 'waiting': return task.blocked_because || 'waiting for you';
            case 'blocked': return 'stopped — ' + (task.blocked_because || 'it could not go on');
            case 'stopped': return 'you stopped it';
            case 'done': return 'finished';
            default: return task.state;
        }
    }

    function stepMark(step) {
        if (step.state === 'done') return step.verdict === 'verified' ? '✓' : '~';
        if (step.state === 'failed') return '✗';
        if (step.state === 'skipped') return '–';
        if (step.state === 'needs_you') return '!';
        if (step.state === 'running') return '…';
        return '·';
    }

    /*
     * Verified and claimed are shown differently, and said out loud.
     *
     * "I have written the file" is not evidence that a file exists. A tick
     * that means both is a tick that means nothing, so a claimed step gets its
     * own mark and its own words — and on a machine with one small model,
     * somebody can see for themselves how much of a report is which.
     */
    function verdictWords(step) {
        // A step that failed outright says why. It has no verdict — nothing
        // got as far as being checked — and a bare ✗ with no reason beside it
        // is the least useful thing on the page.
        if (step.state === 'failed' && step.why) return step.why;
        if (step.state === 'skipped') return step.why || 'skipped';
        if (step.verdict === 'verified') {
            return 'checked against what the tools returned'
                + (step.checked_by ? ' · ' + step.checked_by : '');
        }
        if (step.verdict === 'claimed') return 'its own word for it — nothing was looked up';
        if (step.verdict === 'unmet') return step.why || 'it did not do what the step was for';
        return '';
    }

    /*
     * The level as a tag, or nothing.
     *
     * quietFrom is the least serious level worth drawing at all: in a list of
     * tasks only high and critical earn the space, while the detail of one
     * task names every level so nothing about it is left unsaid.
     */
    const LEVELS = ['low', 'medium', 'high', 'critical'];

    function riskTag(level, quietFrom) {
        const at = LEVELS.indexOf(level || 'low');

        if (at < LEVELS.indexOf(quietFrom)) return null;

        const tag = document.createElement('span');
        tag.className = 'risk-tag risk-' + LEVELS[Math.max(at, 0)];
        tag.textContent = LEVELS[Math.max(at, 0)] + ' risk';
        return tag;
    }

    function taskRow(task, withButtons) {
        const row = document.createElement('div');
        row.className = 'task-row';

        const name = document.createElement('button');
        name.type = 'button';
        name.className = 'task-name';
        name.textContent = task.name;
        name.addEventListener('click', () => { open = task.id; refresh(); });
        row.appendChild(name);

        const state = document.createElement('span');
        state.className = 'task-state';
        state.textContent = stateOf(task);
        row.appendChild(state);

        const tag = riskTag(task.risk, 'high');
        if (tag) row.insertBefore(tag, state);

        if (!withButtons) return row;

        if (task.state === 'working' || task.state === 'planning') {
            row.appendChild(button('Stop', () => act(task.id, 'stop')));
        }

        // Only offered when nothing is actually in the way. A task parked on a
        // decision is picked up by making the decision, and a Resume button
        // beside it would be a button that does nothing.
        if (task.state === 'waiting' && !/decision/.test(task.blocked_because || '')) {
            row.appendChild(button('Pick it up', () => act(task.id, 'resume')));
        }

        return row;
    }

    function button(label, onClick) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'task-button';
        b.textContent = label;
        b.addEventListener('click', onClick);
        return b;
    }

    async function act(id, what) {
        try {
            const res = await fetch('/api/tasks/' + id + '/' + what, { method: 'POST' });
            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say(body.error || 'That did not work.');
                return;
            }
            refresh();
        } catch (err) {
            say('That did not work: ' + err.message);
        }
    }

    async function start() {
        const box = el('task-request');
        if (!box || !box.value.trim()) return;

        say('Planning it…');

        try {
            const res = await fetch('/api/tasks', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ request: box.value.trim() })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say(body.error || 'That did not work.');
                return;
            }

            box.value = '';
            typing = false;
            open = body.id;
            say('Started: ' + body.name);
            refresh();
        } catch (err) {
            say('That did not work: ' + err.message);
        }
    }

    function fill(node, tasks, withButtons, empty) {
        node.textContent = '';

        if (!tasks.length) {
            const p = document.createElement('p');
            p.className = 'note';
            p.textContent = empty;
            node.appendChild(p);
            return;
        }

        for (const task of tasks) node.appendChild(taskRow(task, withButtons));
    }

    function detail(task) {
        const card = el('task-detail-card');
        if (!card) return;

        if (!task) { card.hidden = true; return; }

        card.hidden = false;
        el('task-detail-name').textContent = task.name;

        const stateLine = el('task-detail-state');
        stateLine.textContent = stateOf(task) + ' ';
        stateLine.appendChild(riskTag(task.risk, 'low'));

        /*
         * The report below the state, not appended to it.
         *
         * Appended, a stopped task read "stopped — the model is not running ·
         * Stopped on X — the model is not running." The state line and the
         * report's first line say the same thing by design, and saying it
         * twice in one sentence reads like the program stuttering.
         */
        const report = el('task-detail-report');
        if (report) {
            const rest = (task.report || '').split('\n').slice(1).join('\n').trim();
            report.textContent = rest;
            report.hidden = rest === '';
        }

        const steps = el('task-detail-steps');
        steps.textContent = '';

        for (const step of (task.steps || [])) {
            const row = document.createElement('div');
            row.className = 'task-step';

            const mark = document.createElement('span');
            mark.className = 'task-step-mark';
            mark.textContent = stepMark(step);
            row.appendChild(mark);

            const body = document.createElement('div');
            body.className = 'task-step-body';

            const what = document.createElement('div');
            what.className = 'task-step-what';
            what.textContent = step.instruction;

            const stepTag = riskTag(step.risk, 'medium');
            if (stepTag) what.appendChild(stepTag);

            body.appendChild(what);

            /*
             * What ran at high or critical without a question.
             *
             * Only ever on never stop. Listed on the step as well as in the
             * report, so the one that moved money can be found where it
             * happened rather than only in a paragraph at the end.
             */
            if (step.acted) {
                const acted = document.createElement('ul');
                acted.className = 'task-acted';

                for (const line of step.acted.split('\n')) {
                    if (!line.trim()) continue;
                    const li = document.createElement('li');
                    li.textContent = 'done without asking: ' + line;
                    acted.appendChild(li);
                }

                body.appendChild(acted);
            }

            const words = verdictWords(step);
            if (words) {
                const note = document.createElement('div');
                note.className = 'task-step-note';
                note.textContent = words;
                body.appendChild(note);
            }

            /*
             * Who did it, then what they used.
             *
             * That order on purpose: when a step comes back wrong the first
             * question is which of five agents produced it, and the model
             * name only matters once that is known.
             */
            if (step.assignee || step.model) {
                const who = document.createElement('div');
                who.className = 'task-step-note';
                who.textContent = [step.assignee, step.model].filter(Boolean).join(' · ');
                body.appendChild(who);
            }

            if (step.evidence) {
                const shown = document.createElement('details');
                const summary = document.createElement('summary');
                summary.textContent = 'What came back';
                shown.appendChild(summary);

                const pre = document.createElement('pre');
                pre.className = 'task-evidence';
                pre.textContent = step.evidence;
                shown.appendChild(pre);
                body.appendChild(shown);
            }

            row.appendChild(body);
            steps.appendChild(row);
        }
    }

    /*
     * The strip on the command centre.
     *
     * One line per running task, and hidden entirely when there are none — a
     * card that always says "nothing" teaches people not to look at it.
     */
    function strip(live) {
        const node = el('tasks-now');
        if (!node) return;

        node.textContent = '';
        node.hidden = live.length === 0;

        for (const task of live) {
            const row = document.createElement('button');
            row.type = 'button';
            row.className = 'tasks-now-row';
            row.textContent = task.name + ' — ' + stateOf(task);
            row.addEventListener('click', () => {
                open = task.id;
                document.querySelector('.nav-item[data-view="tasks"]')?.click();
                refresh();
            });
            node.appendChild(row);
        }

        const count = el('nav-tasks');
        if (count) {
            count.textContent = String(live.length);
            count.hidden = live.length === 0;
        }
    }

    async function refresh() {
        let body;

        try {
            const res = await fetch('/api/tasks');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const live = body.tasks || [];
        const recent = (body.recent || []).filter(
            (t) => !live.some((l) => l.id === t.id));

        strip(live);

        const liveNode = el('task-live');
        const recentNode = el('task-recent');

        if (liveNode) fill(liveNode, live, true, 'Nothing running.');
        if (recentNode) fill(recentNode, recent, true, 'Nothing yet.');

        if (!open) { detail(null); return; }

        try {
            const res = await fetch('/api/tasks/' + open);
            detail(res.ok ? await res.json() : null);
        } catch (err) {
            detail(null);
        }
    }

    function wire() {
        el('task-start')?.addEventListener('click', start);

        const box = el('task-request');
        if (box) {
            box.addEventListener('input', () => { typing = box.value.length > 0; });
            box.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') { e.preventDefault(); start(); }
            });
        }

        refresh();

        // Joined to the same five seconds everything else on this page uses.
        setInterval(() => { if (!typing) refresh(); }, 5000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
