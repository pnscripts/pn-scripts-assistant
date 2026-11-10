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
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    // A key's words when it has them, the given text when it does not.
    function maybe(key, otherwise, vars) {
        const said = T(key, vars);

        return said === key ? otherwise : said;
    }

    /*
     * A specialist's name in the page's language.
     *
     * Steps and the weekly review name a specialist by its file name
     * ("developer"); the built-in ones have their names in i18n.js, and one
     * somebody added keeps the name it was given.
     */
    function agentTitle(name) {
        if (!name) return '';

        return maybe('task.agent.' + String(name).toLowerCase(), name);
    }

    // The same, from the English title the program wrote into a sentence
    // ("waiting on the game developer"), through the roster team.js reads.
    function titled(title) {
        const name = (window.pnTeamByTitle || {})[String(title || '').toLowerCase()];

        return name ? agentTitle(name) : title;
    }

    /*
     * Why a task is waiting or stopped, in the page's language.
     *
     * The reason is kept on the task in English, as the program wrote it, and
     * the server sends a key beside it when the program worded it itself
     * (tasks.ReasonKey). A reason that carries an error from somewhere else
     * keeps that error's words.
     */
    function reason(task) {
        const key = task.blocked_key;
        const args = Object.assign({}, task.blocked_args || {});

        if (!key) return task.blocked_because || '';

        if (args.why_key) args.why = T(args.why_key);
        if (args.who) args.who = titled(args.who);

        return T(key, args);
    }

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
            case 'planning': return T('task.st.planning');
            case 'working': return T('task.st.working');
            case 'waiting': return task.blocked_because ? reason(task) : T('task.st.waiting');
            case 'blocked': return T('task.st.blocked', {
                why: task.blocked_because ? reason(task) : T('task.couldNotGoOn')
            });
            case 'stopped': return T('task.st.stopped');
            case 'done': return T('task.st.done');
            default: return task.state;
        }
    }

    // For the room, which lists the same tasks.
    window.pnTaskState = stateOf;
    window.pnAgentTitle = agentTitle;

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
        if (step.state === 'skipped') return step.why || T('task.skipped');
        if (step.verdict === 'verified') {
            return T('task.checked')
                + (step.checked_by ? ' · ' + agentTitle(step.checked_by) : '');
        }
        if (step.verdict === 'claimed') return T('task.claimed');
        if (step.verdict === 'unmet') return step.why || T('task.unmet');
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
        tag.className = 'risk-tag';
        tag.classList.add(`risk-${LEVELS[Math.max(at, 0)]}`);
        tag.textContent = T('task.riskTag', { level: T('risk.' + LEVELS[Math.max(at, 0)]) });
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
            row.appendChild(button(T('task.stop'), () => act(task.id, 'stop')));
        }

        // Only offered when nothing is actually in the way. A task parked on a
        // decision is picked up by making the decision, and a Resume button
        // beside it would be a button that does nothing.
        if (task.state === 'waiting' && !/decision/.test(task.blocked_because || '')) {
            row.appendChild(button(T('task.pickUp'), () => act(task.id, 'resume')));
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
                say(body.error ? T('task.failedWhy', { why: body.error }) : T('task.failed'));
                return;
            }
            refresh();
        } catch (err) {
            say(T('task.failedWhy', { why: err.message }));
        }
    }

    async function start() {
        const box = el('task-request');
        if (!box || !box.value.trim()) return;

        say(T('task.planning'));

        try {
            const res = await fetch('/api/tasks', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ request: box.value.trim() })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say(body.error ? T('task.failedWhy', { why: body.error }) : T('task.failed'));
                return;
            }

            box.value = '';
            typing = false;
            open = body.id;
            say(T('task.started', { name: body.name }));
            refresh();
        } catch (err) {
            say(T('task.failedWhy', { why: err.message }));
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

        // What it can prove it did: commands, files, checks, pictures.
        if (window.pnTaskEvidence) window.pnTaskEvidence(task.id);

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

        /*
         * Where the work came from and who held it.
         *
         * A specialist's task names the job it is a step of, and every task
         * names the people who did any part of it — the team this job formed,
         * which is the only team most jobs ever have.
         */
        const chain = el('task-detail-chain');
        if (chain) {
            chain.textContent = '';

            if (task.parent_task_id) {
                const up = document.createElement('button');
                up.type = 'button';
                up.className = 'task-link';
                up.textContent = T('task.stepOf', { n: task.parent_task_id });
                up.addEventListener('click', () => { open = task.parent_task_id; refresh(); });
                chain.append(...around(T('task.thisIs'), up));
            }

            if ((task.team || []).length) {
                chain.append(T('task.workedOnBy', { who: task.team.map(agentTitle).join(', ') }));
            }

            chain.hidden = chain.textContent === '';
        }

        const positionOf = {};
        for (const s of (task.steps || [])) positionOf[s.id] = s.position;

        const childOf = {};
        for (const c of (task.children || [])) childOf[c.id] = c;

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

            // How this step is joined to the others and to other people.
            if (step.escalated_from || step.review_of) {
                const link = document.createElement('div');
                link.className = 'task-step-note';
                link.textContent = step.escalated_from
                    ? T('task.sentUp', { n: positionOf[step.escalated_from] || '?' })
                    : T('task.reviewOf', { n: positionOf[step.review_of] || '?' });
                body.appendChild(link);
            }

            if (step.handed_to) {
                const child = childOf[step.handed_to];
                const link = document.createElement('div');
                link.className = 'task-step-note';

                const open_ = document.createElement('button');
                open_.type = 'button';
                open_.className = 'task-link';
                open_.textContent = child ? child.name : T('task.taskN', { n: step.handed_to });
                open_.addEventListener('click', () => { open = step.handed_to; refresh(); });

                link.append(...around(T('task.handedOn'), open_), child ? ' (' + stateOf(child) + ')' : '');
                body.appendChild(link);
            }

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
                    li.textContent = T('task.doneWithoutAsking', { line: line });
                    acted.appendChild(li);
                }

                body.appendChild(acted);
            }

            if (step.state === 'running' && waitingFor['step:' + step.id]) {
                const queued = document.createElement('div');
                queued.className = 'task-step-note';
                queued.textContent = waitingFor['step:' + step.id];
                body.appendChild(queued);
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
                who.textContent = [agentTitle(step.assignee), step.model].filter(Boolean).join(' · ');
                // Why this model, as the router said it, on hover.
                if (step.model_why) who.title = step.model_why;
                body.appendChild(who);
            }

            if (step.evidence) {
                const shown = document.createElement('details');
                const summary = document.createElement('summary');
                summary.textContent = T('task.cameBack');
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
            row.textContent = task.name + ': ' + stateOf(task);
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

    /*
     * The lanes, in a sentence each.
     *
     * "Waiting" on its own is the word that makes queued work look stuck, so
     * a lane says who has it and who is behind them, in the order they will
     * go. And the steps that are waiting are remembered, so the task detail
     * can say the same beside the step itself.
     */
    let waitingFor = {};

    // Who holds or waits for a lane: the specialist's own name when the
    // server says which one it is.
    function seatName(seat) {
        return seat.agent ? agentTitle(seat.agent) : seat.who;
    }

    // A sentence with an element in it: the words either side of {link}.
    function around(sentence, node) {
        const [before, after] = sentence.split('{link}');

        return [before, node, after || ''].filter((part) => part !== '');
    }

    function drawLanes(lanes) {
        waitingFor = {};

        const node = el('task-lanes');
        if (!node) return;

        node.textContent = '';

        for (const lane of lanes) {
            const inIt = lane.in || [];
            const behind = lane.waiting || [];

            behind.forEach((seat, i) => {
                if (!seat.key) return;
                const holders = inIt.map((s) => seatName(s).toLowerCase()).join(T('task.and'));
                waitingFor[seat.key] = maybe('task.waitFor.' + lane.name, T('task.waitForLane', { lane: lane.name })) +
                    (holders ? T(inIt.length > 1 ? 'task.heldByMany' : 'task.heldBy', { who: holders }) : '') +
                    (i > 0 ? T('task.ahead', { n: i }) : '');
            });

            if (!inIt.length && !behind.length) continue;

            const row = document.createElement('p');
            row.className = 'task-lane';

            const name = document.createElement('b');
            name.textContent = maybe('task.lane.' + lane.name, lane.name[0].toUpperCase() + lane.name.slice(1)) +
                (lane.size > 1 ? T('task.ofSize', { n: inIt.length, size: lane.size }) : '');
            row.appendChild(name);

            row.append(': ' + (inIt.length
                ? inIt.map((s) => seatName(s) + ': ' + s.what).join('; ')
                : T('task.free')));

            if (behind.length) {
                row.append(T('task.waitingInOrder', { who: behind.map(seatName).join(', ') }));
            }

            node.appendChild(row);
        }

        node.hidden = node.childElementCount === 0;
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
        drawLanes(body.lanes || []);

        const liveNode = el('task-live');
        const recentNode = el('task-recent');

        if (liveNode) fill(liveNode, live, true, T('room.nothingRunning'));
        if (recentNode) fill(recentNode, recent, true, T('teach.nothingYet'));

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

        // The cards are rebuilt; what somebody is typing is not touched.
        window.addEventListener('pn-language', refresh);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
