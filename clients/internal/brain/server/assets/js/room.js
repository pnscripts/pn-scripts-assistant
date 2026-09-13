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
        what.textContent = item.summary || item.tool;
        row.appendChild(what);

        const tool = document.createElement('div');
        tool.className = 'task-step-note';
        tool.textContent = item.tool;
        row.appendChild(tool);

        const buttons = document.createElement('div');
        buttons.className = 'agent-controls';
        buttons.appendChild(button('Approve', () => decide(item.id, true)));
        buttons.appendChild(button('Reject', () => decide(item.id, false)));
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

    function whenText(goal) {
        if (goal.state !== 'active') return goal.state;
        if (!goal.every_days) return 'when you ask';
        if (goal.due) return 'due now';

        if (goal.next_due) {
            const days = Math.round((new Date(goal.next_due) - Date.now()) / 86400000);
            if (days <= 0) return 'due now';
            return 'in ' + days + (days === 1 ? ' day' : ' days');
        }

        return 'every ' + goal.every_days + ' days';
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
        when.textContent = whenText(goal) + (goal.starts_itself ? ' · starts itself' : '');
        row.appendChild(when);

        row.appendChild(button('Work on it', () => workOn(goal.id)));
        row.appendChild(button('Forget', () => forget(goal.id)));

        // What has actually been done towards it. A goal with no history is a
        // note somebody wrote, and saying so plainly is more use than a blank.
        const history = document.createElement('div');
        history.className = 'agent-note';

        const tasks = goal.tasks || [];

        history.textContent = tasks.length
            ? tasks.map((t) => t.name + ' — ' + t.state).join(' · ')
            : 'nothing done towards it yet';

        row.appendChild(history);

        return row;
    }

    async function workOn(id) {
        say('goal-said', 'Starting…');

        try {
            const res = await fetch('/api/goals/' + id + '/work', { method: 'POST' });
            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('goal-said', body.error || 'That did not start.');
                return;
            }

            say('goal-said', 'Started: ' + (body.name || 'it'));
            refresh();
        } catch (err) {
            say('goal-said', 'That did not start: ' + err.message);
        }
    }

    async function forget(id) {
        try {
            const res = await fetch('/api/goals/' + id, { method: 'DELETE' });

            if (!res.ok) return;

            say('goal-said', 'Forgotten. What was done towards it is still recorded.');
            refresh();
        } catch (err) {
            /* the next poll will show it either way */
        }
    }

    async function saveGoal() {
        const name = el('goal-name')?.value.trim();

        if (!name) {
            say('goal-said', 'Say what the goal is.');
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
                say('goal-said', body.error || 'That did not save.');
                return;
            }

            el('goal-name').value = '';
            el('goal-why').value = '';
            el('goal-auto').checked = false;

            const details = el('goal-new');
            if (details) details.open = false;

            say('goal-said', 'Set.');
            refresh();
        } catch (err) {
            say('goal-said', 'That did not save: ' + err.message);
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
            list.appendChild(note('Nothing set yet. A goal is something that comes '
                + 'round again rather than something that finishes.'));
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
            list.appendChild(note('Nothing running.'));
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
            state.textContent = task.state === 'waiting'
                ? (task.blocked_because || 'waiting for you')
                : task.state;
            row.appendChild(state);

            row.appendChild(button('Watch it', () => {
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
        say('habits-said', 'Saving…');

        try {
            const res = await fetch('/api/permissions/decide', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    tool: tool,
                    answer: answer,
                    why: 'you had answered this the same way every time'
                })
            });

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say('habits-said', body.error || 'That did not save.');
                return;
            }

            say('habits-said', answer === 'allow'
                ? 'It will stop asking about ' + tool + '. You can take that back in Permissions.'
                : 'It will always refuse ' + tool + '. You can take that back in Permissions.');

            loadHabits();
        } catch (err) {
            say('habits-said', 'That did not save: ' + err.message);
        }
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
            name.textContent = habit.tool;
            row.appendChild(name);

            const said = document.createElement('span');
            said.className = 'task-state';
            said.textContent = habit.said;
            row.appendChild(said);

            row.appendChild(button(
                habit.suggest === 'allow' ? 'Stop asking' : 'Always refuse',
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

            node.textContent = gone
                ? 'It has stopped believing ' + gone + (gone === 1 ? ' thing: ' : ' things: ')
                  + (memory.superseded || 0) + ' replaced by a newer reading, '
                  + (memory.retired || 0) + ' whose source is gone.'
                : 'It has not had to unlearn anything yet.';
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
            part.className = 'review-part review-' + kind;
            part.style.width = (count / total * 100) + '%';
            part.title = count + ' ' + kind;
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
            summary.appendChild(note('No work has been done in the last week.'));
            el('review-people').textContent = '';
            return;
        }

        const line = document.createElement('p');
        line.className = 'note';

        const parts = [
            (tasks.done || 0) + ' finished',
            (tasks.blocked || 0) + ' stopped'
        ];

        if (tasks.waiting) parts.push(tasks.waiting + ' waiting for you');
        if (tasks.running) parts.push(tasks.running + ' still going');

        line.textContent = parts.join(', ') + ' · '
            + (steps.verified || 0) + ' steps checked against what the tools returned, '
            + (steps.claimed || 0) + ' taken on its own word, '
            + (steps.unmet || 0) + ' that did not do what they were for'
            + (steps.failed ? ', ' + steps.failed + ' that could not run at all' : '')
            + '.';
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
            name.textContent = person.name;
            row.appendChild(name);

            const what = document.createElement('span');
            what.className = 'task-state';
            what.textContent = person.steps + ' steps · ' + person.verified + ' checked, '
                + person.claimed + ' claimed'
                + (person.failed ? ', ' + person.failed + ' failed' : '')
                + ' · ' + Math.round(person.seconds) + 's';
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
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
