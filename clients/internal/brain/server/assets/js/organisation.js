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
        }

        return row;
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

            for (const unit of body.units || []) {
                list.appendChild(unitBlock(unit));

                for (const seat of unit.seats || []) {
                    seats.push({ name: seat.name, title: seat.title, unit: unit.title });
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
                }

                list.appendChild(loose);
            }

            el('org-counts').textContent = body.jobs + ' jobs and ' + body.capabilities
                + ' capabilities to hire from';

            el('org-folders').textContent = 'Chart: ' + body.folder + ' · People: ' + body.agents;

            fillSeats();
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

        said.textContent = 'taking them on…';

        try {
            const res = await fetch('/api/organisation/agents', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: el('org-hire-name').value.trim(),
                    job: chosen.id,
                    position: el('org-hire-seat').value,
                    uses: el('org-hire-uses').value,
                }),
            });

            const body = await res.json();

            if (!res.ok) {
                said.textContent = body.error || 'could not take them on';

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

    function typing(node, run) {
        let waiting = null;

        node.addEventListener('input', () => {
            clearTimeout(waiting);
            waiting = setTimeout(run, 250);
        });
    }

    function wire() {
        document.querySelector('.nav-item[data-view="organisation"]')
            ?.addEventListener('click', load);

        const search = el('org-job-search');

        if (search) typing(search, findJobs);

        const can = el('org-who-can');

        if (can) typing(can, whoKnows);

        el('org-hire')?.addEventListener('click', hire);

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
