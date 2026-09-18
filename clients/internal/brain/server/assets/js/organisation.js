(function () {
    'use strict';

    /*
     * The organisation: the chart, who holds each seat, and the work there is
     * in the world to be held.
     *
     * Read when the view is opened rather than on a timer. Nothing here
     * changes on its own — a chart changes when somebody edits it — and a poll
     * over a form would only ever be a way to clear a field somebody was
     * halfway through typing into.
     */

    const el = (id) => document.getElementById(id);

    let chosen = null;
    let seats = [];
    let templates = [];
    let names = new Set();

    function say(text) {
        const node = el('org-said');

        if (node) node.textContent = text || '';
    }

    function line(className, text) {
        const node = document.createElement('p');

        node.className = className;
        node.textContent = text;

        return node;
    }

    function heldBy(seat) {
        const holders = seat.held_by || [];

        if (!holders.length) return 'nobody yet';

        return holders.map((a) => a.title || a.name).join(', ');
    }

    function seatRow(seat) {
        const row = document.createElement('div');

        row.className = 'org-seat' + (seat.vacant ? ' org-vacant' : '');

        const head = document.createElement('div');

        head.className = 'org-seat-head';

        const title = document.createElement('span');

        title.className = 'org-seat-title';
        title.textContent = seat.title || seat.name;
        head.appendChild(title);

        if (seat.seniority) {
            const rank = document.createElement('span');

            rank.className = 'org-rank';
            rank.textContent = seat.seniority;
            head.appendChild(rank);
        }

        row.appendChild(head);

        /*
         * Only said when there is nobody, because the lines below name
         * whoever there is. Said both ways round, the chart read "Game
         * developer / Game developer / Game developer" — the seat, the
         * person and the job, all the same word, which is what happens when
         * a panel prints every field it has.
         */
        if (seat.vacant) row.appendChild(line('org-held', heldBy(seat)));

        const job = seat.job_title || seat.job;

        if (job && job.toLowerCase() !== (seat.title || '').toLowerCase()) {
            row.appendChild(line('note', job));
        }

        if (seat.reports_to) {
            row.appendChild(line('note', 'answers to ' + seat.reports_to));
        }

        /*
         * What each agent in the seat may actually touch.
         *
         * The interesting line, and the reason a chart is worth looking at:
         * two agents in one seat can have entirely different authority, and
         * this is where that is visible rather than implied.
         */
        for (const agent of seat.held_by || []) {
            const what = [];

            if (agent.model) what.push(agent.model);
            if (agent.provider) what.push('through ' + agent.provider);

            const tools = agent.tools || [];

            what.push(tools.length > 3
                ? tools.length + ' tools'
                : tools.join(', '));

            if ((agent.never || []).length) {
                what.push('never ' + agent.never.join(', '));
            }

            if (agent.state && agent.state !== 'active') what.push(agent.state);

            row.appendChild(line('org-agent', (agent.title || agent.name) + ' — ' + what.join(' · ')));
            row.appendChild(agentLife(agent));
        }

        return row;
    }

    /*
     * What somebody is doing, what they have done, and the few things that
     * can be done to them from here.
     *
     * Everything else — tools, capabilities, manner — is a file somebody can
     * open, and a form for every field would be a second, worse editor.
     */
    function agentLife(agent) {
        const box = document.createElement('div');

        box.className = 'org-life';

        const facts = [];

        if ((agent.doing || []).length) {
            facts.push('working on: ' + agent.doing.join('; '));
        } else if (agent.state === 'active' || !agent.state || agent.state === 'temporary') {
            facts.push('free');
        }

        if (agent.waiting) facts.push(agent.waiting + ' queued');

        if (agent.hired_for) facts.push('hired for task ' + agent.hired_for + ' only');

        const done = agent.record || {};

        if (done.steps) {
            facts.push(done.steps + (done.steps === 1 ? ' step' : ' steps') + ' this month, '
                + done.verified + ' checked, ' + done.claimed + ' on its own word');
        }

        // How the work went between people: none of these is a verdict on its
        // own, and all of them are worth being able to see.
        if (done.handed_on) facts.push('handed on ' + done.handed_on);
        if (done.escalated) facts.push(done.escalated + ' sent up');
        if (done.found_wrong) facts.push(done.found_wrong + ' found wrong on review');

        box.appendChild(line('note', facts.join(' · ')));

        /*
         * What it remembers, opened on request.
         *
         * Its own, and not the job's: two agents in one seat remember
         * different things, which is the point of there being two of them.
         */
        if (agent.remembers) {
            const memory = document.createElement('details');
            const summary = document.createElement('summary');

            summary.className = 'note';
            summary.textContent = 'remembers ' + agent.remembers
                + (agent.remembers === 1 ? ' thing' : ' things') + ' from its own work';
            memory.appendChild(summary);

            memory.addEventListener('toggle', async () => {
                if (!memory.open || memory.dataset.loaded) return;

                memory.dataset.loaded = '1';

                try {
                    const res = await fetch('/api/organisation/memories/' + encodeURIComponent(agent.name));
                    const body = await res.json();

                    for (const m of body.memories || []) {
                        memory.appendChild(line('note',
                            (m.kind === 'learned' ? 'lesson — ' : '') + m.content));
                    }
                } catch {
                    memory.appendChild(line('note', 'could not read them'));
                }
            });

            box.appendChild(memory);
        }

        if (agent.built_in && !agent.state) return box;

        const actions = document.createElement('div');

        actions.className = 'org-actions';

        const working = !agent.state || agent.state === 'active' || agent.state === 'temporary';

        actions.appendChild(action(working ? 'Suspend' : 'Activate', () =>
            setState(agent.name, working ? 'suspended' : 'active')));

        if (agent.state === 'temporary') {
            actions.appendChild(action('Keep them', () => setState(agent.name, 'active')));
        }

        actions.appendChild(action('Copy', () => copyOf(agent.name)));

        box.appendChild(actions);

        return box;
    }

    function action(label, run) {
        const b = document.createElement('button');

        b.type = 'button';
        b.className = 'task-button';
        b.textContent = label;
        b.addEventListener('click', run);

        return b;
    }

    async function send(url, body) {
        const res = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });

        const answer = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(answer.error || 'that did not work');

        return answer;
    }

    async function setState(name, state) {
        try {
            await send('/api/organisation/agents/' + encodeURIComponent(name) + '/state', { state });
            say(name + ' is ' + (state === 'active' ? 'working here' : state) + ' now');
            load();
        } catch (err) {
            say(err.message);
        }
    }

    // A copy is a new name, the same everything else, and nothing linking the
    // two: changing one later changes nobody else.
    async function copyOf(name) {
        let copy = name + '_2';

        for (let i = 3; names.has(copy); i++) copy = name + '_' + i;

        try {
            const body = await send('/api/organisation/agents', { name: copy, clone_of: name });
            if (body.id) {
                proposed(body, { set textContent(text) { say(text); } });
            } else {
                say(body.hired + ' is a copy of ' + name);
                load();
            }
        } catch (err) {
            say(err.message);
        }
    }

    /*
     * Proposing, then hiring.
     *
     * "Propose somebody" writes nothing: it shows who would be hired, what
     * they could touch, what the machine would need and who else would be
     * needed. The hire is made by the button its owner presses under that —
     * permanently, or for one task, which then starts.
     */
    let proposal = null;

    function showProposal(p) {
        proposal = p;

        const box = el('org-proposal');
        const existing = !!(p.proposal && p.proposal.existing);

        box.hidden = false;
        el('org-proposal-text').textContent = p.text || '';

        const open = p.state === 'open' && !existing;

        // Taken on from the catalogue or copied: already a permanent hire,
        // so the only choices are to confirm it or not.
        const permanent = !!(p.proposal && p.proposal.permanence === 'permanent');

        el('org-proposal-permanent').hidden = !open;
        el('org-proposal-task').hidden = !open || permanent;
        el('org-proposal-decline').hidden = !open;

        const goal = (p.proposal && p.proposal.goal) || '';

        el('org-proposal-goal').value = goal;
        el('org-proposal-goal-field').hidden = !open || permanent;
    }

    // A proposal for somebody new, from any button: shown where hiring is
    // decided, and nothing written until it is confirmed there.
    function proposed(p, said) {
        said.textContent = 'Proposal ' + p.id + ' — nothing is hired until you confirm it under “Hire someone”.';
        showProposal(p);
        el('org-proposal').scrollIntoView({ behavior: 'smooth', block: 'center' });
    }

    async function hireFor() {
        const sentence = el('org-hire-for').value.trim();
        const said = el('org-hire-for-said');

        if (!sentence) return;

        said.textContent = 'working out who…';

        try {
            const p = await send('/api/organisation/hire', { sentence });

            said.textContent = (p.proposal && p.proposal.existing)
                ? 'Somebody here already does this.'
                : 'Proposal ' + p.id + '. Nothing is hired until you say so below.';

            showProposal(p);
        } catch (err) {
            said.textContent = err.message;
        }
    }

    async function confirmHire(permanence) {
        if (!proposal) return;

        const said = el('org-hire-for-said');
        const goal = el('org-proposal-goal').value.trim();

        if (permanence === 'task' && !goal) {
            said.textContent = 'Say what the one task is, above the buttons.';
            el('org-proposal-goal').focus();

            return;
        }

        try {
            const body = await send('/api/organisation/hire/' + proposal.id + '/confirm', { permanence, goal });

            said.textContent = body.said;
            el('org-proposal').hidden = true;
            el('org-hire-for').value = '';
            load();
        } catch (err) {
            said.textContent = err.message;
        }
    }

    async function declineHire() {
        if (!proposal) return;

        try {
            await send('/api/organisation/hire/' + proposal.id + '/decline', {});
            el('org-hire-for-said').textContent = 'Nobody was hired.';
            el('org-proposal').hidden = true;
        } catch (err) {
            el('org-hire-for-said').textContent = err.message;
        }
    }

    function unitBlock(unit) {
        const block = document.createElement('div');

        block.className = 'org-unit';
        block.style.marginLeft = Math.min(unit.depth, 4) * 18 + 'px';

        const head = document.createElement('div');

        head.className = 'org-unit-head';

        const title = document.createElement('span');

        title.className = 'org-unit-title';
        title.textContent = unit.title || unit.name;
        head.appendChild(title);

        const kind = document.createElement('span');

        kind.className = 'org-kind';
        kind.textContent = unit.kind;
        head.appendChild(kind);

        if (!unit.built_in) {
            const changed = document.createElement('span');

            changed.className = 'org-kind';
            changed.textContent = 'yours';
            head.appendChild(changed);
        }

        block.appendChild(head);

        if (unit.purpose) block.appendChild(line('note', unit.purpose));

        for (const seat of unit.seats || []) block.appendChild(seatRow(seat));

        return block;
    }

    async function load() {
        const list = el('org-chart');

        if (!list) return;

        try {
            const res = await fetch('/api/organisation');
            const body = await res.json();

            if (!res.ok) {
                say(body.error || 'could not read the organisation');

                return;
            }

            list.textContent = '';
            seats = [];
            names = new Set((body.unseated || []).map((a) => a.name));

            for (const unit of body.units || []) {
                list.appendChild(unitBlock(unit));

                for (const seat of unit.seats || []) {
                    seats.push({ name: seat.name, title: seat.title, unit: unit.title });

                    for (const a of seat.held_by || []) names.add(a.name);
                }
            }

            if (!(body.units || []).length) {
                list.appendChild(line('note', 'Nothing yet.'));
            }

            if ((body.unseated || []).length) {
                const loose = document.createElement('div');

                loose.className = 'org-unit';
                loose.appendChild(line('org-unit-title', 'Holding no seat'));
                loose.appendChild(line('note',
                    'They work exactly as they always did. A seat is something to give '
                    + 'them, not something they are missing.'));

                for (const agent of body.unseated) {
                    loose.appendChild(line('org-agent', (agent.title || agent.name)
                        + ' — ' + (agent.for || '')));
                    loose.appendChild(agentLife(agent));
                }

                list.appendChild(loose);
            }

            el('org-counts').textContent = body.jobs + ' jobs and ' + body.capabilities
                + ' capabilities to hire from';

            el('org-folders').textContent = 'Chart: ' + body.folder + ' · People: ' + body.agents;

            templates = body.templates || [];

            fillSeats();
            fillTemplates();
        } catch {
            say('could not reach the brain');
        }
    }

    function fillSeats() {
        const picker = el('org-hire-seat');

        if (!picker) return;

        const was = picker.value;

        picker.textContent = '';

        const none = document.createElement('option');

        none.value = '';
        none.textContent = 'no seat — just somebody who does this';
        picker.appendChild(none);

        for (const seat of seats) {
            const option = document.createElement('option');

            option.value = seat.name;
            option.textContent = seat.title + ' · ' + seat.unit;
            picker.appendChild(option);
        }

        picker.value = was;
    }

    function fillTemplates() {
        const picker = el('org-hire-template');

        if (!picker) return;

        const was = picker.value;

        picker.textContent = '';

        const none = document.createElement('option');

        none.value = '';
        none.textContent = 'the job alone';
        picker.appendChild(none);

        for (const t of templates) {
            const option = document.createElement('option');

            option.value = t.name;
            option.textContent = t.title + (t.built_in ? '' : ' (yours)');
            picker.appendChild(option);
        }

        picker.value = was;
    }

    function jobRow(job) {
        const row = document.createElement('div');

        row.className = 'org-job';

        const head = document.createElement('div');

        head.className = 'org-seat-head';

        const title = document.createElement('span');

        title.className = 'org-seat-title';
        title.textContent = job.title;
        head.appendChild(title);

        if (job.risk && job.risk !== 'low') {
            const risk = document.createElement('span');

            risk.className = 'org-risk org-risk-' + job.risk;
            risk.textContent = job.risk;
            head.appendChild(risk);
        }

        if (job.oversight) {
            const watched = document.createElement('span');

            watched.className = 'org-risk';
            watched.textContent = 'needs a person';
            head.appendChild(watched);
        }

        if (job.status && job.status !== 'established') {
            const status = document.createElement('span');

            status.className = 'org-kind';
            status.textContent = job.status;
            head.appendChild(status);
        }

        row.appendChild(head);

        if (job.what) row.appendChild(line('note', job.what));

        const needs = (job.needs || []).map((n) => n.name || n.id);

        if (needs.length) {
            row.appendChild(line('note', 'good at: ' + needs.slice(0, 8).join(', ')));
        }

        row.addEventListener('click', () => choose(job));

        return row;
    }

    function choose(job) {
        chosen = job;

        el('org-hire-card').hidden = false;
        el('org-hire-title').textContent = 'Take on a ' + job.title.toLowerCase();
        el('org-hire-what').textContent = job.what || '';
        el('org-hire-said').textContent = '';

        const suggested = job.id.split('.').pop();

        if (!el('org-hire-name').value) el('org-hire-name').value = suggested;
    }

    async function findJobs() {
        const looking = el('org-job-search').value.trim();
        const list = el('org-jobs');

        list.textContent = '';

        try {
            const res = await fetch('/api/organisation/jobs?q=' + encodeURIComponent(looking));
            const body = await res.json();

            if (!res.ok) {
                list.appendChild(line('note', body.error || 'could not look'));

                return;
            }

            if (!(body.jobs || []).length) {
                list.appendChild(line('note', 'Nothing here does that yet.'));

                return;
            }

            for (const job of body.jobs) list.appendChild(jobRow(job));
        } catch {
            list.appendChild(line('note', 'could not reach the brain'));
        }
    }

    async function hire() {
        if (!chosen) return;

        const said = el('org-hire-said');

        said.textContent = 'working out the proposal…';

        try {
            const res = await fetch('/api/organisation/agents', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: el('org-hire-name').value.trim(),
                    job: chosen.id,
                    position: el('org-hire-seat').value,
                    uses: el('org-hire-uses').value,
                    template: el('org-hire-template').value,
                }),
            });

            const body = await res.json();

            if (!res.ok) {
                said.textContent = body.error || 'could not take them on';

                return;
            }

            if (body.id) {
                proposed(body, said);

                return;
            }

            said.textContent = body.was_already_here
                ? body.hired + ' was already here, and has been changed'
                : body.hired + ' works here now';

            el('org-hire-name').value = '';

            load();
        } catch {
            said.textContent = 'could not reach the brain';
        }
    }

    async function whoKnows() {
        const can = el('org-who-can').value.trim();
        const list = el('org-who');

        list.textContent = '';

        if (!can) return;

        try {
            const res = await fetch('/api/organisation/who?can=' + encodeURIComponent(can));
            const body = await res.json();

            if (!res.ok) {
                list.appendChild(line('note', body.error || 'could not look'));

                return;
            }

            if ((body.who || []).length) {
                for (const one of body.who) {
                    list.appendChild(line('org-agent',
                        (one.title || one.name) + (one.position ? ' · ' + one.position : '')));
                }
            } else {
                list.appendChild(line('note', 'Nobody here, yet.'));
            }

            /*
             * And what would have to be taken on, which is a better answer
             * than no. The point of a taxonomy of jobs nobody holds is being
             * able to name the one that is missing.
             */
            if ((body.jobs || []).length) {
                list.appendChild(line('note', 'Jobs that call for it: '
                    + body.jobs.map((j) => j.title).join(', ')));
            }
        } catch {
            list.appendChild(line('note', 'could not reach the brain'));
        }
    }

    /*
     * Imports: what is running and how far it has got, and what the last few
     * did. Asked every two seconds only while one is running — an import is
     * minutes, and a view that polls forever over nothing is the kind of cost
     * this program has gone out of its way not to have.
     */
    let importPoll = null;

    function counted(n, one, many) {
        n = n || 0;

        return n + ' ' + (n === 1 ? one : (many || one + 's'));
    }

    async function imports() {
        const list = el('org-imports');

        if (!list) return;

        let body;

        try {
            const res = await fetch('/api/organisation/imports');
            body = await res.json();
        } catch {
            return;
        }

        list.textContent = '';

        const runs = body.imports || [];

        if (!runs.length) {
            list.appendChild(line('note', 'Nothing imported yet.'));
        }

        let running = false;

        for (const run of runs) {
            const so = run.so_far || {};
            const what = run.source === 'onet' ? 'O*NET' : 'ESCO';

            let said = what + ' — ' + run.state;

            if (run.state === 'running') {
                running = true;
                said += ': ' + (run.stage || 'starting') + (run.of ? ' ' + run.done + ' of ' + run.of
                    : run.done ? ' ' + run.done : '');
            }

            said += ' · ' + [
                counted(so.jobs, 'new job'), counted(so.capabilities, 'new capability', 'new capabilities'),
                counted(so.links, 'link'), (so.filled || 0) + ' filled in',
            ].join(', ');

            if (run.error) said += ' · ' + run.error;

            const row = line('org-agent', said);
            list.appendChild(row);

            if (run.state === 'running') {
                list.appendChild(action('Stop', async () => {
                    try {
                        await send('/api/organisation/imports/' + run.id + '/stop', {});
                    } catch (err) {
                        say(err.message);
                    }
                    imports();
                }));
            }

            for (const note of run.notes || []) list.appendChild(line('note', note));
        }

        clearTimeout(importPoll);

        if (running) {
            importPoll = setTimeout(imports, 2000);
        } else if (runs.length && runs[0].state === 'done') {
            load();
        }
    }

    async function startImport() {
        const from = el('org-import-from').value.trim();

        if (!from) return;

        try {
            await send('/api/organisation/import', { source: el('org-import-source').value, from });
            imports();
        } catch (err) {
            el('org-imports').textContent = '';
            el('org-imports').appendChild(line('note', err.message));
        }
    }

    function typing(node, run) {
        let waiting = null;

        node.addEventListener('input', () => {
            clearTimeout(waiting);
            waiting = setTimeout(run, 250);
        });
    }

    function wire() {
        document.querySelector('.nav-item[data-view="organisation"]')
            ?.addEventListener('click', () => { load(); imports(); });

        el('org-import-go')?.addEventListener('click', startImport);

        const search = el('org-job-search');

        if (search) typing(search, findJobs);

        const can = el('org-who-can');

        if (can) typing(can, whoKnows);

        el('org-hire')?.addEventListener('click', hire);
        el('org-hire-for-go')?.addEventListener('click', hireFor);
        el('org-proposal-permanent')?.addEventListener('click', () => confirmHire('permanent'));
        el('org-proposal-task')?.addEventListener('click', () => confirmHire('task'));
        el('org-proposal-decline')?.addEventListener('click', declineHire);
        el('org-hire-for')?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') { e.preventDefault(); hireFor(); }
        });

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
