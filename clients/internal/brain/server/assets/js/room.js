/*
 * Everything it is doing, and what all of it is for.
 *
 * The command centre answers "what is happening this second". This answers the
 * question somebody actually opens the program with: what is it working on, is
 * any of it waiting for me, and has any of it been any good.
 *
 * Polled, like the rest of this interface, and for the reason console.js gives:
 * a decision can be made from another window, and a few seconds of staleness
 * costs nothing here. Nothing in this room changes faster than a step, and a
 * step is minutes.
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

    function say(id, text) {
        const node = el(id);
        if (node) node.textContent = text || '';
    }

    function note(text) {
        const p = document.createElement('p');
        p.className = 'note';
        p.textContent = text;
        return p;
    }

    function button(label, onClick) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'task-button';
        b.textContent = label;
        b.addEventListener('click', onClick);
        return b;
    }

    /* ---------- waiting for you ---------- */

    async function decide(id, approve) {
        try {
            const res = await fetch('/api/approvals/' + id + '/' + (approve ? 'approve' : 'deny'), {
                method: 'POST'
            });

            if (!res.ok) return;

            refresh();
        } catch (err) {
            /* the next poll will show it either way */
        }
    }

    function waitingRow(item) {
        const row = document.createElement('div');
        row.className = 'approval';

        const what = document.createElement('div');
        what.className = 'task-step-what';

        // textContent, never markup: a summary is written by a tool and this
        // is the one place somebody is deciding whether to trust it.
        what.textContent = item.summary || toolName(item.tool);
        row.appendChild(what);

        const tool = document.createElement('div');
        tool.className = 'task-step-note';
        tool.textContent = toolName(item.tool);
        row.appendChild(tool);

        const buttons = document.createElement('div');
        buttons.className = 'agent-controls';
        buttons.appendChild(button(T('room.approve'), () => decide(item.id, true)));
        buttons.appendChild(button(T('room.reject'), () => decide(item.id, false)));
        row.appendChild(buttons);

        return row;
    }

    async function loadWaiting() {
        let items = [];

        try {
            const res = await fetch('/api/approvals');
            if (res.ok) {
                // A bare array, which is what the rest of the interface counts.
                const body = await res.json();
                items = Array.isArray(body) ? body : (body.approvals || []);
            }
        } catch (err) {
            return;
        }

        const card = el('room-waiting-card');
        const list = el('room-waiting');
        if (!card || !list) return;

        card.hidden = items.length === 0;
        list.textContent = '';

        for (const item of items) list.appendChild(waitingRow(item));

        const count = el('nav-room');
        if (count) {
            count.textContent = String(items.length);
            count.hidden = items.length === 0;
        }
    }

    /* ---------- what it is for ---------- */

    function toolName(name) {
        return window.pnToolName ? window.pnToolName(name) : name;
    }

    // A task's state in a word, as the task list says it.
    function taskState(task) {
        if (window.pnTaskState) return window.pnTaskState(task);

        return task.state;
    }

    function whenText(goal) {
        if (goal.state !== 'active') return maybe(`room.state.${goal.state}`, goal.state);
        if (!goal.every_days) return T('room.whenAsk');
        if (goal.due) return T('room.dueNow');

        if (goal.next_due) {
            const days = Math.round((new Date(goal.next_due) - Date.now()) / 86400000);
            if (days <= 0) return T('room.dueNow');
            return days === 1 ? T('room.inDay') : T('room.inDays', { n: days });
        }

        return T('room.everyDays', { n: goal.every_days });
    }

    function goalRow(goal) {
        const row = document.createElement('div');
        row.className = 'task-row';

        const name = document.createElement('span');
        name.className = 'agent-title';
        name.textContent = goal.name;
        row.appendChild(name);

        const when = document.createElement('span');
        when.className = 'task-state';
        when.textContent = whenText(goal) + (goal.starts_itself ? ' · ' + T('room.startsItself') : '');
        row.appendChild(when);

        row.appendChild(button(T('room.workOn'), () => workOn(goal.id)));
        row.appendChild(button(T('room.forget'), () => forget(goal.id)));

        // What has actually been done towards it. A goal with no history is a
        // note somebody wrote, and saying so plainly is more use than a blank.
        const history = document.createElement('div');
        history.className = 'agent-note';

        const tasks = goal.tasks || [];

        history.textContent = tasks.length
            ? tasks.map((t) => t.name + ': ' + taskState(t)).join(' · ')
            : T('room.nothingDone');

        row.appendChild(history);

        return row;
    }

    async function workOn(id) {
        say('goal-said', T('room.starting'));

        try {
            const res = await fetch('/api/goals/' + id + '/work', { method: 'POST' });
            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('goal-said', body.error ? T('room.notStartedWhy', { why: body.error }) : T('room.notStarted'));
                return;
            }

            say('goal-said', T('room.started', { name: body.name || '' }));
            refresh();
        } catch (err) {
            say('goal-said', T('room.notStartedWhy', { why: err.message }));
        }
    }

    async function forget(id) {
        try {
            const res = await fetch('/api/goals/' + id, { method: 'DELETE' });

            if (!res.ok) return;

            say('goal-said', T('room.forgotten'));
            refresh();
        } catch (err) {
            /* the next poll will show it either way */
        }
    }

    async function saveGoal() {
        const name = el('goal-name')?.value.trim();

        if (!name) {
            say('goal-said', T('room.sayGoal'));
            return;
        }

        try {
            const res = await fetch('/api/goals', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: name,
                    why: el('goal-why')?.value.trim() || '',
                    every_days: Number(el('goal-every')?.value || 0),
                    starts_itself: !!el('goal-auto')?.checked
                })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('goal-said', body.error ? T('room.notSavedWhy', { why: body.error }) : T('room.notSaved'));
                return;
            }

            el('goal-name').value = '';
            el('goal-why').value = '';
            el('goal-auto').checked = false;

            const details = el('goal-new');
            if (details) details.open = false;

            say('goal-said', T('room.set'));
            refresh();
        } catch (err) {
            say('goal-said', T('room.notSavedWhy', { why: err.message }));
        }
    }

    // No guard here either: this rebuilds the list of goals, which holds no
    // fields. See diary.js — the guard only ever stopped the list updating at
    // the moment somebody was about to want it updated.
    async function loadGoals() {
        let body;

        try {
            const res = await fetch('/api/goals');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const list = el('goal-list');
        if (!list) return;

        list.textContent = '';

        const goals = body.goals || [];

        if (!goals.length) {
            list.appendChild(note(T('room.noGoals')));
            return;
        }

        for (const goal of goals) list.appendChild(goalRow(goal));
    }

    /* ---------- going on now ---------- */

    async function loadLive() {
        let body;

        try {
            const res = await fetch('/api/tasks');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const list = el('room-live');
        if (!list) return;

        list.textContent = '';

        const live = body.tasks || [];

        if (!live.length) {
            list.appendChild(note(T('room.nothingRunning')));
            return;
        }

        for (const task of live) {
            const row = document.createElement('div');
            row.className = 'task-row';

            const name = document.createElement('span');
            name.className = 'agent-title';
            name.textContent = task.name;
            row.appendChild(name);

            const state = document.createElement('span');
            state.className = 'task-state';
            state.textContent = taskState(task);
            row.appendChild(state);

            row.appendChild(button(T('room.watch'), () => {
                document.querySelector('.nav-item[data-view="tasks"]')?.click();
            }));

            list.appendChild(row);
        }
    }

    /* ---------- things it keeps asking about ---------- */

    /*
     * A suggestion, and only a suggestion.
     *
     * Agreeing writes a standing answer through the same endpoint the
     * permissions panel uses, so there is one place that decides what the
     * assistant may do and one record of it. Widening that on its own is the
     * change this program must never make.
     */
    async function settle(tool, answer) {
        say('habits-said', T('room.saving'));

        try {
            const res = await fetch('/api/permissions/decide', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    tool: tool,
                    answer: answer,
                    why: T('room.habitWhy')
                })
            });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say('habits-said', body.error ? T('room.notSavedWhy', { why: body.error }) : T('room.notSaved'));
                return;
            }

            say('habits-said', T(answer === 'allow' ? 'room.willStopAsking' : 'room.willRefuse',
                { tool: toolName(tool) }));

            loadHabits();
        } catch (err) {
            say('habits-said', T('room.notSavedWhy', { why: err.message }));
        }
    }

    // What the record says, in the page's language: from the counts the
    // server sends beside its own English sentence.
    function times(n) {
        if (n === 1) return T('room.once');
        if (n === 2) return T('room.twice');

        return T('room.times', { n: n });
    }

    function habitSaid(habit) {
        return habit.suggest === 'allow'
            ? T('room.habitYes', { times: times(habit.approved || 0) })
            : T('room.habitNo', { times: times(habit.refused || 0) });
    }

    async function loadHabits() {
        let body;

        try {
            const res = await fetch('/api/habits');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const card = el('habits-card');
        const list = el('habits-list');
        if (!card || !list) return;

        const noticed = body.noticed || [];

        card.hidden = noticed.length === 0;
        list.textContent = '';

        for (const habit of noticed) {
            const row = document.createElement('div');
            row.className = 'task-row';

            const name = document.createElement('span');
            name.className = 'skill-name';
            name.textContent = toolName(habit.tool);
            row.appendChild(name);

            const said = document.createElement('span');
            said.className = 'task-state';
            said.textContent = habitSaid(habit);
            row.appendChild(said);

            row.appendChild(button(
                T(habit.suggest === 'allow' ? 'room.stopAsking' : 'room.alwaysRefuse'),
                () => settle(habit.tool, habit.suggest)));

            list.appendChild(row);
        }
    }

    /* ---------- what it has stopped believing ---------- */

    /*
     * The other half of getting sharper, and on its own rather than inside the
     * review.
     *
     * It was inside it, after an early return for "no work this week" — so on
     * a machine that had done nothing yet it never appeared at all, which is
     * exactly the machine where somebody is looking for signs of life. What it
     * has unlearned is about memory, not about work, and does not depend on
     * whether any work has been done.
     */
    async function loadForgotten() {
        const node = el('review-forgotten');
        if (!node) return;

        try {
            const res = await fetch('/api/knowledge?limit=1');
            if (!res.ok) return;

            const memory = await res.json();
            const gone = (memory.superseded || 0) + (memory.retired || 0);

            const counts = { n: gone, replaced: memory.superseded || 0, gone: memory.retired || 0 };

            node.textContent = gone
                ? T(gone === 1 ? 'room.unlearnedOne' : 'room.unlearnedMany', counts)
                : T('room.unlearnedNone');
        } catch (err) {
            /* a nicety; the rest of the pane stands without it */
        }
    }

    /* ---------- how it has been going ---------- */

    function bar(verified, claimed, unmet) {
        const total = verified + claimed + unmet;
        const wrap = document.createElement('div');
        wrap.className = 'review-bar';

        if (!total) return wrap;

        for (const [count, kind] of [[verified, 'verified'], [claimed, 'claimed'], [unmet, 'unmet']]) {
            if (!count) continue;

            const part = document.createElement('span');
            part.className = 'review-part';
            part.classList.add(`review-${kind}`);
            part.style.width = (count / total * 100) + '%';
            part.title = T(`room.kind.${kind}`, { n: count });
            wrap.appendChild(part);
        }

        return wrap;
    }

    async function loadReview() {
        let body;

        try {
            const res = await fetch('/api/review?days=7');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const review = body.review || {};
        const steps = review.steps || {};
        const tasks = review.tasks || {};

        const summary = el('review-summary');
        if (!summary) return;

        summary.textContent = '';

        /*
         * Anything at all counts as something to report.
         *
         * Counting only steps that reached a verdict, a week in which the
         * model was unreachable and every task stopped read as "nothing to
         * report" — which is flattery by omission, and the exact thing this
         * pane exists to prevent.
         */
        const anySteps = (steps.verified || 0) + (steps.claimed || 0)
            + (steps.unmet || 0) + (steps.failed || 0) + (steps.skipped || 0);
        const anyTasks = (tasks.done || 0) + (tasks.blocked || 0) + (tasks.stopped || 0)
            + (tasks.running || 0) + (tasks.waiting || 0);

        if (!anySteps && !anyTasks) {
            summary.appendChild(note(T('room.noWork')));
            el('review-people').textContent = '';
            return;
        }

        const line = document.createElement('p');
        line.className = 'note';

        const parts = [
            T('room.finished', { n: tasks.done || 0 }),
            T('room.stopped', { n: tasks.blocked || 0 })
        ];

        if (tasks.waiting) parts.push(T('room.waitingN', { n: tasks.waiting }));
        if (tasks.running) parts.push(T('room.stillGoing', { n: tasks.running }));

        const stepParts = [
            T('room.stepsChecked', { n: steps.verified || 0 }),
            T('room.stepsClaimed', { n: steps.claimed || 0 }),
            T('room.stepsUnmet', { n: steps.unmet || 0 })
        ];

        if (steps.failed) stepParts.push(T('room.stepsFailed', { n: steps.failed }));

        line.textContent = parts.join(', ') + ' · ' + stepParts.join(', ') + '.';
        summary.appendChild(line);

        /*
         * The bar is about the quality of work that happened.
         *
         * A step that could not run has no quality to show, so a week of those
         * gets the sentence above and no bar at all — rather than an empty box
         * that looks like a bar which has failed to draw.
         */
        if ((steps.verified || 0) + (steps.claimed || 0) + (steps.unmet || 0) > 0) {
            summary.appendChild(bar(steps.verified || 0, steps.claimed || 0, steps.unmet || 0));
        }

        const people = el('review-people');
        people.textContent = '';

        for (const person of (review.people || [])) {
            const row = document.createElement('div');
            row.className = 'task-row';

            const name = document.createElement('span');
            name.className = 'agent-title';
            name.textContent = window.pnAgentTitle ? window.pnAgentTitle(person.name) : person.name;
            row.appendChild(name);

            const what = document.createElement('span');
            what.className = 'task-state';
            what.textContent = T('room.person', { steps: person.steps, checked: person.verified, claimed: person.claimed })
                + (person.failed ? ', ' + T('room.personFailed', { n: person.failed }) : '')
                + ' · ' + T('room.seconds', { n: Math.round(person.seconds) });
            row.appendChild(what);

            people.appendChild(row);
        }
    }

    function refresh() {
        loadWaiting();
        loadGoals();
        loadLive();
        loadReview();
        loadHabits();
        loadForgotten();
    }

    function wire() {
        el('goal-save')?.addEventListener('click', saveGoal);

        refresh();

        // The same five seconds the rest of the interface uses. Waiting is
        // polled whatever view is open, so the count beside the nav is right
        // without somebody having to be looking at the room.
        setInterval(() => {
            loadWaiting();

            const view = document.querySelector('section.view[data-view="room"]');
            if (view && !view.hidden) {
                loadGoals();
                loadLive();
                loadReview();
                loadHabits();
                loadForgotten();
            }
        }, 5000);

        document.querySelector('.nav-item[data-view="room"]')
            ?.addEventListener('click', refresh);

        window.addEventListener('pn-language', refresh);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
