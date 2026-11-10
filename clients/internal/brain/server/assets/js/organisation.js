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
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    // A string of i18n.js when there is one, and what the server said when
    // there is not: names the owner wrote are his, in his words.
    function named(key, fallback) {
        const said = T(key);

        return said === key ? fallback : said;
    }

    function toolName(name) {
        return window.pnToolName ? window.pnToolName(name) : String(name || '').replace(/_/g, ' ');
    }

    // The chart, the people and the templates are said in the page's
    // language while they are called what they shipped as (the server says
    // which); a name the owner changed or wrote keeps his words.
    const unitTitle = (u) => (u.shipped ? named('org.unit.' + u.name, u.title) : (u.title || u.name));
    const unitPurpose = (u) => (u.shipped_purpose ? named('org.unitPurpose.' + u.name, u.purpose) : u.purpose);
    const seatTitle = (seat) => (seat.shipped ? named('org.seat.' + seat.name, seat.title) : (seat.title || seat.name));
    const whoTitle = (a) => (a.shipped ? named('org.who.' + a.name, a.title) : (a.title || a.name));
    // What a built-in specialist is for, in the page's language while it still
    // says what it shipped with (team.go, the same words as on Tasks).
    const whoFor = (a) => {
        const key = 'task.agentFor.' + a.name;

        return window.pnEnglish && a.for && window.pnEnglish(key) === a.for ? T(key) : (a.for || '');
    };
    const kindWord = (kind) => named('org.kind.' + kind, kind);
    const rankWord = (rank) => named('org.rank.' + rank, rank);
    const stateWord = (state) => named('org.state.' + state, state);
    const riskWord = (risk) => named('org.risk.' + risk, risk);

    let chosen = null;
    let seatTitles = {};
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

        if (!holders.length) return T('org.nobodyYet');

        return holders.map(whoTitle).join(', ');
    }

    function seatRow(seat) {
        const shownTitle = seatTitle(seat);
        const row = document.createElement('div');

        row.className = 'org-seat' + (seat.vacant ? ' org-vacant' : '');

        const head = document.createElement('div');

        head.className = 'org-seat-head';

        const title = document.createElement('span');

        title.className = 'org-seat-title';
        title.textContent = shownTitle;
        head.appendChild(title);

        if (seat.seniority) {
            const rank = document.createElement('span');

            rank.className = 'org-rank';
            rank.textContent = rankWord(seat.seniority);
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
            row.appendChild(line('note', jobTitle({ id: seat.job, title: job, title_bg: seat.job_title_bg })));
        }

        if (seat.reports_to) {
            row.appendChild(line('note', T('org.answersTo', { who: seatTitles[seat.reports_to] || seat.reports_to })));
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
            if (agent.provider) what.push(T('org.through', { provider: agent.provider }));

            what.push(toolsWords(agent.tools || []));

            if ((agent.never || []).length) {
                what.push(T('org.never', { list: agent.never.map(toolName).join(', ') }));
            }

            if (agent.state && agent.state !== 'active') what.push(stateWord(agent.state));

            row.appendChild(line('org-agent', whoTitle(agent) + ': ' + what.join(' · ')));
            row.appendChild(agentLife(agent));
        }

        return row;
    }

    // The server says "everything" and "none: its work is judgement" rather
    // than an empty list, which means two opposite things.
    function toolsWords(tools) {
        if (tools.length === 1 && tools[0] === 'everything') return T('org.toolsAll');
        if (tools.length === 1 && tools[0].startsWith('none:')) return T('org.toolsNone');
        if (tools.length > 3) return T('org.toolsMany', { n: tools.length });

        return tools.map(toolName).join(', ');
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
            facts.push(T('org.workingOn', { what: agent.doing.join('; ') }));
        } else if (agent.state === 'active' || !agent.state || agent.state === 'temporary') {
            facts.push(T('org.free'));
        }

        if (agent.waiting) facts.push(T('org.queued', { n: agent.waiting }));

        if (agent.hired_for) facts.push(T('org.hiredFor', { id: agent.hired_for }));

        const done = agent.record || {};

        if (done.steps) {
            facts.push(T(done.steps === 1 ? 'org.recordOne' : 'org.recordMany',
                { n: done.steps, verified: done.verified || 0, claimed: done.claimed || 0 }));
        }

        // How the work went between people: none of these is a verdict on its
        // own, and all of them are worth being able to see.
        if (done.handed_on) facts.push(T('org.handedOn', { n: done.handed_on }));
        if (done.escalated) facts.push(T('org.escalated', { n: done.escalated }));
        if (done.found_wrong) facts.push(T('org.foundWrong', { n: done.found_wrong }));

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
            summary.textContent = T(agent.remembers === 1 ? 'org.remembersOne' : 'org.remembersMany',
                { n: agent.remembers });
            memory.appendChild(summary);

            memory.addEventListener('toggle', async () => {
                if (!memory.open || memory.dataset.loaded) return;

                memory.dataset.loaded = '1';

                try {
                    const res = await fetch('/api/organisation/memories/' + encodeURIComponent(agent.name));
                    const body = await res.json();

                    for (const m of body.memories || []) {
                        memory.appendChild(line('note',
                            m.kind === 'learned' ? T('org.lesson', { text: m.content }) : m.content));
                    }
                } catch {
                    memory.appendChild(line('note', T('org.couldNotReadThem')));
                }
            });

            box.appendChild(memory);
        }

        if (agent.built_in && !agent.state) return box;

        const actions = document.createElement('div');

        actions.className = 'org-actions';

        const working = !agent.state || agent.state === 'active' || agent.state === 'temporary';

        actions.appendChild(action(T(working ? 'org.suspend' : 'org.activate'), () =>
            setState(agent.name, working ? 'suspended' : 'active')));

        if (agent.state === 'temporary') {
            actions.appendChild(action(T('org.keepThem'), () => setState(agent.name, 'active')));
        }

        actions.appendChild(action(T('org.copy'), () => copyOf(agent.name)));

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

        if (!res.ok) throw new Error(answer.error || T('org.failed'));

        return answer;
    }

    async function setState(name, state) {
        try {
            await send('/api/organisation/agents/' + encodeURIComponent(name) + '/state', { state });
            say(T(state === 'active' ? 'org.nowActive' : 'org.nowSuspended', { name }));
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
                say(T('org.copyOf', { copy: body.hired, name }));
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

    /*
     * The proposal in the page's language, from what it is made of rather
     * than from the server's English sentence of it. What came from data
     * (the job, its packages, what each needs and why) stays as written.
     */
    function proposalText(row) {
        const p = row.proposal;

        if (!p) return row.text || '';

        if (p.existing) {
            return T('org.p.existing', { title: whoTitle(p.existing), name: p.existing.name });
        }

        const a = p.agent || {};
        const job = p.job || {};
        const out = [T('org.p.hire', { title: a.title || a.name, name: a.name })];
        let jobLine = T('org.p.job', { title: job.title || '', id: job.id || '' });

        if (p.seat) {
            const where = p.seat_in
                ? T('org.p.newSeat', { unit: p.seat_unit || p.seat_in })
                : T('org.p.vacantSeat');

            jobLine += T('org.p.sitting', { where, seat: p.seat });
        }

        out.push(jobLine);

        if (p.permanence === 'permanent') {
            out.push(T('org.p.permanent'));
        } else if (p.permanence === 'task') {
            out.push(T('org.p.taskOnly'));
            if (p.goal) out.push(T('org.p.theTask', { goal: p.goal }));
        } else {
            out.push(T('org.p.undecided'));
        }

        out.push(T('org.p.risk', { risk: riskWord(p.risk) }));

        const section = (key, lines) => {
            if (!lines || !lines.length) return;

            out.push('', T(key));

            for (const l of lines) out.push('  · ' + l);
        };

        section('org.p.responsible', p.responsibilities);
        section('org.p.packages', (p.packages || []).map((pkg) => {
            let l = pkg.id + ' ' + pkg.version + ': ' + pkg.title + ' (' + named('org.work.' + pkg.work, pkg.work) + ')';

            if ((pkg.needs || []).length) l += T('org.p.reliesOn', { list: pkg.needs.join('; ') });

            return l;
        }));

        const m = p.model || {};
        const uses = named('org.usesShort.' + m.uses, m.uses);
        const model = m.name ? T('org.p.modelNamed', { name: m.name, uses }) : T('org.p.model', { uses });

        section('org.p.thinksWith', [T('org.p.on', { model, provider: m.provider || '' })]);

        if (a.brief) section('org.p.brief', [a.brief.replace(/\n/g, ' ')]);

        section('org.p.skills', p.skills);
        section('org.p.mayUse', [(p.tools || []).length ? p.tools.map(toolName).join(', ') : T('org.toolsNone')]);
        section('org.p.mustNot', p.limits);
        section('org.p.needs', (p.requires || []).map(neededLine));
        section('org.p.integrations', (p.integrations || []).map((i) => {
            const st = i.state || {};
            const state = st.approved ? T('org.p.intApproved') : st.known ? T('org.p.intKnown') : T('org.p.intUnlisted');
            const where = st.remote ? T('org.p.intOutside') : T('org.p.intHere');

            return (st.title || i.id) + ': ' + i.why + '; ' + where + ', ' + state;
        }));
        section('org.p.itself', p.itself);
        section('org.p.you', p.you);
        section('org.p.professional', p.professional);

        return out.join('\n');
    }

    // One requirement as it stands on this machine, as team.neededLine says it.
    function neededLine(n) {
        const s = n.status || {};
        const ready = s.present && s.compatible && (!s.licence_state || s.licence_state === 'active');
        let l = (s.title || '') + (s.wanted ? ' ' + s.wanted : '');

        if (ready) l += T('org.need.ready', { version: s.version || T('org.need.noVersion') });
        else if (s.present) l += T('org.need.hereBut', { problem: s.problem || '' });
        else if (s.blocked) l += ': ' + (s.problem || '');
        else l += T('org.need.missing');

        l += ' [' + (n.optional ? T('org.need.optional', { why: n.why }) : n.why) + ']';

        if (!ready) {
            const terms = [s.size, s.source, s.licence, s.cost].filter(Boolean);

            if (terms.length) l += '; ' + terms.join('; ');
            if (!s.installable && s.manual) l += T('org.need.manual', { manual: s.manual });
        }

        return l;
    }

    function showProposal(p) {
        proposal = p;

        const box = el('org-proposal');
        const existing = !!(p.proposal && p.proposal.existing);

        box.hidden = false;
        el('org-proposal-text').textContent = proposalText(p);

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
        said.textContent = T('org.proposalConfirmAbove', { id: p.id });
        showProposal(p);
        el('org-proposal').scrollIntoView({ behavior: 'smooth', block: 'center' });
    }

    async function hireFor() {
        const sentence = el('org-hire-for').value.trim();
        const said = el('org-hire-for-said');

        if (!sentence) return;

        said.textContent = T('org.workingOutWho');

        try {
            const p = await send('/api/organisation/hire', { sentence });

            said.textContent = (p.proposal && p.proposal.existing)
                ? T('org.alreadyHere')
                : T('org.proposalSayBelow', { id: p.id });

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
            said.textContent = T('org.sayTheTask');
            el('org-proposal-goal').focus();

            return;
        }

        try {
            await send('/api/organisation/hire/' + proposal.id + '/confirm', { permanence, goal });

            said.textContent = hiredWords(proposal, permanence, goal);
            el('org-proposal').hidden = true;
            el('org-hire-for').value = '';
            load();
        } catch (err) {
            said.textContent = err.message;
        }
    }

    // What the server said in English, said from the proposal confirmed.
    function hiredWords(row, permanence, goal) {
        const p = row.proposal || {};
        const a = p.agent || {};
        const vars = { name: a.name || '', title: a.title || a.name || '', seat: seatTitles[p.seat] || p.seat || '', goal };

        if (permanence === 'task') return T('org.hiredTask', vars);

        return T(p.seat ? 'org.hiredPermanentSeat' : 'org.hiredPermanent', vars);
    }

    async function declineHire() {
        if (!proposal) return;

        try {
            await send('/api/organisation/hire/' + proposal.id + '/decline', {});
            el('org-hire-for-said').textContent = T('org.nobodyHired');
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
        title.textContent = unitTitle(unit);
        head.appendChild(title);

        const kind = document.createElement('span');

        kind.className = 'org-kind';
        kind.textContent = kindWord(unit.kind);
        head.appendChild(kind);

        if (!unit.built_in) {
            const changed = document.createElement('span');

            changed.className = 'org-kind';
            changed.textContent = T('org.yours');
            head.appendChild(changed);
        }

        block.appendChild(head);

        if (unit.purpose) block.appendChild(line('note', unitPurpose(unit)));

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
                say(body.error || T('org.cannotRead'));

                return;
            }

            list.textContent = '';
            seats = [];
            seatTitles = {};
            names = new Set((body.unseated || []).map((a) => a.name));

            for (const unit of body.units || []) {
                for (const seat of unit.seats || []) {
                    seatTitles[seat.name] = seatTitle(seat);
                }
            }

            for (const unit of body.units || []) {
                list.appendChild(unitBlock(unit));

                for (const seat of unit.seats || []) {
                    seats.push({ name: seat.name, title: seatTitles[seat.name], unit: unitTitle(unit) });

                    for (const a of seat.held_by || []) names.add(a.name);
                }
            }

            if (!(body.units || []).length) {
                list.appendChild(line('note', T('org.nothingYet')));
            }

            if ((body.unseated || []).length) {
                const loose = document.createElement('div');

                loose.className = 'org-unit';
                loose.appendChild(line('org-unit-title', T('org.noSeat')));
                loose.appendChild(line('note', T('org.noSeatNote')));

                for (const agent of body.unseated) {
                    loose.appendChild(line('org-agent', whoTitle(agent)
                        + ': ' + whoFor(agent)));
                    loose.appendChild(agentLife(agent));
                }

                list.appendChild(loose);
            }

            el('org-counts').textContent = T('org.counts', { jobs: body.jobs, capabilities: body.capabilities });

            el('org-folders').textContent = T('org.folders', { chart: body.folder, people: body.agents });

            templates = body.templates || [];

            fillSeats();
            fillTemplates();
        } catch {
            say(T('org.noBrain'));
        }
    }

    function fillSeats() {
        const picker = el('org-hire-seat');

        if (!picker) return;

        const was = picker.value;

        picker.textContent = '';

        const none = document.createElement('option');

        none.value = '';
        none.textContent = T('org.noSeatOption');
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
        none.textContent = T('org.jobAlone');
        picker.appendChild(none);

        for (const t of templates) {
            const option = document.createElement('option');

            option.value = t.name;
            option.textContent = t.built_in ? whoTitle(t) : T('org.yoursTemplate', { title: whoTitle(t) });
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
        title.textContent = jobTitle(job);
        head.appendChild(title);

        if (job.risk && job.risk !== 'low') {
            const risk = document.createElement('span');

            risk.className = ['org-risk', 'org-risk-' + job.risk].join(' ');
            risk.textContent = riskWord(job.risk);
            head.appendChild(risk);
        }

        if (job.oversight) {
            const watched = document.createElement('span');

            watched.className = 'org-risk';
            watched.textContent = T('org.needsPerson');
            head.appendChild(watched);
        }

        if (job.status && job.status !== 'established') {
            const status = document.createElement('span');

            status.className = 'org-kind';
            status.textContent = named('org.status.' + job.status, job.status);
            head.appendChild(status);
        }

        row.appendChild(head);

        if (job.what) row.appendChild(line('note', job.what));

        const needs = (job.needs || []).map((n) => n.name || n.id);

        if (needs.length) {
            row.appendChild(line('note', T('org.goodAt', { list: needs.slice(0, 8).join(', ') })));
        }

        row.addEventListener('click', () => choose(job));

        return row;
    }

    // A job's name in Bulgarian, where the classification it came from has one.
    // The jobs the shipped chart's positions stand on are named in i18n.js
    // too, while they still say what they shipped with.
    function jobTitle(job) {
        if (window.pnLang && window.pnLang() === 'bg' && job.title_bg) return job.title_bg;

        const key = 'org.job.' + (job.id || '');
        if (job.id && window.pnEnglish && window.pnEnglish(key) === job.title) return T(key);

        return job.title;
    }

    function choose(job) {
        chosen = job;

        el('org-hire-card').hidden = false;
        el('org-hire-title').textContent = T('org.takeOn', { job: job.title.toLowerCase(), title: jobTitle(job) });
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
                list.appendChild(line('note', body.error || T('org.couldNotLook')));

                return;
            }

            if (!(body.jobs || []).length) {
                list.appendChild(line('note', T('org.noSuchJob')));

                return;
            }

            for (const job of body.jobs) list.appendChild(jobRow(job));
        } catch {
            list.appendChild(line('note', T('org.noBrain')));
        }
    }

    async function hire() {
        if (!chosen) return;

        const said = el('org-hire-said');

        said.textContent = T('org.workingOutProposal');

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
                said.textContent = body.error || T('org.couldNotHire');

                return;
            }

            if (body.id) {
                proposed(body, said);

                return;
            }

            said.textContent = T(body.was_already_here ? 'org.changed' : 'org.worksHereNow', { name: body.hired });

            el('org-hire-name').value = '';

            load();
        } catch {
            said.textContent = T('org.noBrain');
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
                list.appendChild(line('note', body.error || T('org.couldNotLook')));

                return;
            }

            if ((body.who || []).length) {
                for (const one of body.who) {
                    list.appendChild(line('org-agent',
                        whoTitle(one) + (one.position ? ' · ' + (seatTitles[one.position] || one.position) : '')));
                }
            } else {
                list.appendChild(line('note', T('org.nobodyHere')));
            }

            /*
             * And what would have to be taken on, which is a better answer
             * than no. The point of a taxonomy of jobs nobody holds is being
             * able to name the one that is missing.
             */
            if ((body.jobs || []).length) {
                list.appendChild(line('note', T('org.jobsFor', { list: body.jobs.map(jobTitle).join(', ') })));
            }
        } catch {
            list.appendChild(line('note', T('org.noBrain')));
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

        return T(n === 1 ? one : many, { n });
    }

    // The passes an import goes through, as the importers name them, with
    // the spaces in their names written as underscores.
    const stages = {
        skills: 'org.stage.skills', occupations: 'org.stage.occupations',
        what_each_job_needs: 'org.stage.needs', writing: 'org.stage.writing',
        other_titles: 'org.stage.titles', tasks: 'org.stage.tasks',
        knowledge: 'org.stage.knowledge', technologies: 'org.stage.technologies',
    };

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
            list.appendChild(line('note', T('org.noImports')));
        }

        let running = false;

        for (const run of runs) {
            const so = run.so_far || {};
            const what = run.source === 'onet' ? 'O*NET' : 'ESCO';

            let said = what + ': ' + named('org.imp.' + run.state, run.state);

            if (run.state === 'running') {
                running = true;
                said += ': ' + (run.stage ? named(stages[run.stage.replace(/ /g, '_')] || '', run.stage) : T('org.imp.starting'))
                    + (run.of ? ' ' + T('org.imp.of', { done: run.done, of: run.of })
                        : run.done ? ' ' + run.done : '');
            }

            said += ' · ' + [
                counted(so.jobs, 'org.imp.jobsOne', 'org.imp.jobsMany'),
                counted(so.capabilities, 'org.imp.capsOne', 'org.imp.capsMany'),
                counted(so.links, 'org.imp.linksOne', 'org.imp.linksMany'), T('org.imp.filled', { n: so.filled || 0 }),
            ].join(', ');

            if (run.error) said += ' · ' + run.error;

            const row = line('org-agent', said);
            list.appendChild(row);

            if (run.state === 'running') {
                list.appendChild(action(T('org.stop'), async () => {
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

        // Said again in the other language: the chart, the lists, and a
        // proposal on screen.
        window.addEventListener('pn-language', () => {
            load();
            imports();

            if (proposal && !el('org-proposal').hidden) showProposal(proposal);
            if (chosen && !el('org-hire-card').hidden) {
                el('org-hire-title').textContent = T('org.takeOn', { job: chosen.title.toLowerCase(), title: jobTitle(chosen) });
            }

            if (el('org-job-search')?.value.trim()) findJobs();
            if (el('org-who-can')?.value.trim()) whoKnows();
        });

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
