(function () {
    'use strict';

    /*
     * Equipping the organisation: starting a project, the capability packages,
     * what the machine has, and the integrations.
     *
     * Every button here that changes something is its owner's approval, given
     * at the computer — the server refuses these from anywhere else — and each
     * says in its label exactly what it does. Read when the Organisation view
     * is opened, like the rest of it, rather than on a timer.
     */

    const el = (id) => document.getElementById(id);

    function node(tag, className, text) {
        const n = document.createElement(tag);

        if (className) n.className = className;
        if (text !== undefined) n.textContent = text;

        return n;
    }

    function button(label, run) {
        const b = node('button', 'task-button', label);

        b.type = 'button';
        b.addEventListener('click', run);

        return b;
    }

    async function get(url) {
        const res = await fetch(url);
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || 'that did not work');

        return body;
    }

    async function post(url, body) {
        const res = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body || {}),
        });

        const answer = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(answer.error || 'that did not work');

        return answer;
    }

    /* ---------- a project ---------- */

    let project = null;

    function showProject(p) {
        project = p;

        const box = el('proj-proposal');
        const options = el('proj-options');

        box.hidden = false;
        el('proj-proposal-text').textContent = p.text || '';
        options.textContent = '';

        // The engine question answered with a button per engine, each saying
        // what choosing it would take on this machine.
        for (const o of p.options || []) {
            const choice = button(o.title, () => propose(o.package));

            choice.title = o.says;
            options.appendChild(choice);
        }

        const ready = !!p.ready && p.state === 'open';

        el('proj-start').hidden = !ready;
        el('proj-decline').hidden = p.state !== 'open';
    }

    async function propose(pkg) {
        const request = el('proj-request').value.trim();
        const folder = el('proj-folder').value.trim();
        const said = el('proj-said');

        if (!request) {
            said.textContent = 'Say what to make.';

            return;
        }

        said.textContent = 'working it out…';

        try {
            const p = await post('/api/projects/propose', { request, folder, package: pkg || '' });

            said.textContent = p.question ? '' : 'Proposal ' + p.id + '. Read it, then start it or not.';
            showProject(p);
        } catch (err) {
            said.textContent = err.message;
        }
    }

    async function startProject() {
        if (!project) return;

        const said = el('proj-said');

        try {
            const body = await post('/api/projects/' + project.id + '/start');

            said.textContent = body.said;
            el('proj-start').hidden = true;
            el('proj-decline').hidden = true;
        } catch (err) {
            said.textContent = err.message;
        }
    }

    async function declineProject() {
        if (!project) return;

        try {
            await post('/api/projects/' + project.id + '/decline');
            el('proj-said').textContent = 'Left as it was. Nothing was written.';
            el('proj-proposal').hidden = true;
        } catch (err) {
            el('proj-said').textContent = err.message;
        }
    }

    /* ---------- packages ---------- */

    function statusLine(need) {
        const s = need.status || {};
        let text = s.title + (s.wanted ? ' ' + s.wanted : '');

        if (s.present && s.compatible) {
            text += ' — here' + (s.version ? ' (' + s.version + ')' : '');
        } else if (s.present) {
            text += ' — ' + s.problem;
        } else {
            text += ' — not here';
        }

        if (need.optional) text += ' · optional';

        return text;
    }

    async function packages() {
        const list = el('pkg-list');

        if (!list) return;

        try {
            const body = await get('/api/packages');

            list.textContent = '';

            for (const p of body.packages || []) {
                const item = node('details', 'equip-item');
                const head = node('summary', 'equip-head');

                head.append(node('span', 'equip-title', p.title),
                    node('span', 'org-kind', p.work),
                    node('span', 'org-risk org-risk-' + (p.risk || 'low'), p.risk || 'low'));

                item.appendChild(head);
                item.appendChild(node('p', 'note', p.summary));
                item.appendChild(node('p', 'note tiny', p.id + ' ' + p.version + (p.built_in ? ' · ships with the program' : ' · yours: ' + p.file)));

                const role = p.role || {};

                item.appendChild(node('p', 'note', 'Hires a ' + (role.title || '').toLowerCase() + ' who may use: '
                    + ((role.tools || []).join(', ') || 'nothing — judgement only')
                    + ((role.never || []).length ? '; never ' + role.never.join(', ') : '')));

                for (const need of p.requires || []) {
                    item.appendChild(node('p', 'note tiny', 'needs ' + statusLine(need) + ' — ' + need.why));
                }

                for (const limit of p.limits || []) {
                    item.appendChild(node('p', 'note tiny equip-limit', 'must not: ' + limit));
                }

                if (p.escalation) {
                    item.appendChild(node('p', 'note tiny equip-limit', 'a qualified person: ' + p.escalation));
                }

                if ((p.evidence || []).length) {
                    item.appendChild(node('p', 'note tiny', 'finished when there is evidence of: ' + p.evidence.join(', ')));
                }

                list.appendChild(item);
            }

            for (const [file, why] of Object.entries(body.refused || {})) {
                list.appendChild(node('p', 'note equip-limit', file + ' was not used: ' + why));
            }
        } catch (err) {
            list.textContent = err.message;
        }
    }

    /* ---------- what the machine has ---------- */

    async function machine() {
        const list = el('prov-list');

        if (!list) return;

        try {
            const body = await get('/api/provision');

            list.textContent = '';

            for (const s of body.recipes || []) {
                const row = node('div', 'equip-row');
                let here = s.present
                    ? (s.version ? s.version : 'here')
                    : 'not here';

                // Installed is not usable: a licence that will not let it work
                // unattended is said, not hidden behind "here".
                if (s.present && s.licence_state === 'inactive') here += ' — installed, not licensed to work unattended';
                if (s.present && s.licence_state === 'unknown') here += ' — whether it is licensed is not known';

                row.appendChild(node('span', 'equip-title', s.title));
                row.appendChild(node('span', 'note tiny', here + ' · ' + s.kind));

                if (!s.present && s.installable) {
                    row.appendChild(button('Install' + (s.size ? ' (' + s.size + ')' : ''), () => install(s)));
                }

                list.appendChild(row);

                const terms = [s.source, s.licence && 'licence ' + s.licence, s.cost].filter(Boolean).join(' · ');

                if (!s.present) {
                    list.appendChild(node('p', 'note tiny equip-under', s.installable ? terms : (s.manual || terms)));
                }
            }
        } catch (err) {
            list.textContent = err.message;
        }
    }

    async function install(s) {
        const said = el('prov-said');

        try {
            const body = await post('/api/provision/install', { recipe: s.id });

            said.textContent = body.said;
        } catch (err) {
            said.textContent = err.message;
        }
    }

    /* ---------- integrations ---------- */

    async function act(id, action, body) {
        const said = el('int-said');

        try {
            said.textContent = action === 'activate' ? 'starting ' + id + '…' : '';
            await post('/api/integrations/' + encodeURIComponent(id) + '/' + action, body);
            said.textContent = '';
            integrations();
        } catch (err) {
            said.textContent = err.message;
        }
    }

    async function integrations() {
        const list = el('int-list');

        if (!list) return;

        try {
            const body = await get('/api/integrations');

            list.textContent = '';

            for (const v of body.integrations || []) {
                const item = node('div', 'equip-item');
                const head = node('div', 'equip-head');

                let state = 'in the catalogue';

                if (v.running) state = 'running · ' + v.era + ' ' + v.protocol;
                else if (v.approved) state = 'approved · stopped';
                else if (v.listed) state = 'added · not approved';

                head.append(node('span', 'equip-title', v.title), node('span', 'org-kind', state));

                if (v.leaves_the_machine) head.appendChild(node('span', 'org-risk org-risk-high', 'leaves this machine'));

                item.appendChild(head);
                item.appendChild(node('p', 'note', v.why || ''));
                item.appendChild(node('p', 'note tiny', v.where + (v.version ? ' · ' + v.version : '')
                    + (v.licence ? ' · licence ' + v.licence : '')));

                if (v.missing) item.appendChild(node('p', 'note tiny equip-limit', 'needs ' + v.missing + ' installed first'));

                if (v.approved) {
                    item.appendChild(node('p', 'note tiny', 'granted to: ' + ((v.agents || []).join(', ') || 'nobody')));
                }

                for (const t of v.tools || []) {
                    item.appendChild(node('p', 'note tiny', (t.read_only ? 'reads only · ' : 'asks first · ') + t.as
                        + (t.description ? ' — ' + t.description : '') + (t.hint ? ' (' + t.hint + ')' : '')));
                }

                const actions = node('div', 'org-actions');

                if (!v.approved) {
                    actions.appendChild(button('Approve for my assistant', () => act(v.id, 'approve', { agents: ['assistant'] })));
                }

                if (v.approved && !v.running) actions.appendChild(button('Start it', () => act(v.id, 'activate')));
                if (v.running) actions.appendChild(button('Stop it', () => act(v.id, 'deactivate')));
                if (v.approved) actions.appendChild(button('Withdraw approval', () => act(v.id, 'revoke')));
                if (v.listed) actions.appendChild(button('Remove', () => act(v.id, 'remove')));

                item.appendChild(actions);
                list.appendChild(item);
            }
        } catch (err) {
            list.textContent = err.message;
        }
    }

    /* ---------- a task's evidence ---------- */

    // The Tasks view calls this when it opens a task.
    window.pnTaskEvidence = async function (taskID) {
        const box = el('task-detail-evidence');

        if (!box) return;

        try {
            const body = await get('/api/tasks/' + taskID + '/evidence');
            const rows = body.evidence || [];

            box.textContent = '';
            box.hidden = rows.length === 0;

            if (!rows.length) return;

            box.appendChild(node('h3', 'equip-subhead', 'Evidence'));

            for (const e of rows) {
                // The settings as the task found them are for comparing, not
                // for reading: said in a line, never shown as the file.
                if (e.kind === 'settings') {
                    box.appendChild(node('p', 'note tiny', 'settings: the project\'s settings as this task found them'));
                    continue;
                }

                // How the work is being done, and every change of who does
                // it, in full: the reasons are the point.
                if (e.kind === 'decision' || e.kind === 'switch' || e.kind === 'install') {
                    const d = node('details', 'equip-decision');
                    const words = { decision: 'How it is being done', switch: 'Resource changed', install: 'Installed' };

                    d.open = e.kind !== 'install';
                    d.appendChild(node('summary', 'note tiny' + (e.ok || e.kind === 'decision' ? '' : ' equip-limit'),
                        words[e.kind] + ': ' + e.subject));
                    d.appendChild(node('pre', 'proposal-text', e.detail || ''));
                    box.appendChild(d);
                    continue;
                }

                const line = node('p', 'note tiny' + (e.ok ? '' : ' equip-limit'),
                    e.kind + ': ' + e.subject + (e.detail ? ' — ' + e.detail.split('\n')[0] : ''));

                box.appendChild(line);
            }
        } catch (err) {
            box.hidden = true;
        }
    };

    /* ---------- how work is done ---------- */

    const stateWords = {
        available: 'available', degraded: 'working, slowly', low_capacity: 'low on allowance',
        limit_near: 'nearly out of allowance', limit_reached: 'out of allowance', auth_required: 'needs you to sign in',
        not_installed: 'not installed', not_licensed: 'not licensed', not_supported: 'not usable from here',
        offline: 'not answering', failed: 'failing', unknown: 'unknown',
    };

    // An integration nobody has approved is not unusable, only not yet
    // allowed: it is the one reason an integration is listed that way.
    function stateWord(r) {
        if (r.kind === 'integration' && r.state === 'not_supported') return 'not approved';

        return stateWords[r.state] || r.state;
    }

    const gb = 1024 * 1024 * 1024;

    let policy = null;

    function capacity(r) {
        if (!r.usage) return r.kind === 'agent' ? 'capacity unknown' : '';

        const left = r.usage.remaining;
        let text = (left === undefined || left === null) ? 'capacity unknown' : Math.round(left) + '% left';

        if (r.usage.resets_at && !r.usage.resets_at.startsWith('0001')) {
            text += ', resets ' + new Date(r.usage.resets_at).toLocaleString();
        }

        return text;
    }

    function machineLine(m) {
        const ram = m.ram_bytes ? Math.round(m.ram_bytes / gb) + ' GB' : '? GB';
        const parts = [m.os, m.cpu + ' (' + m.cores + ' cores)', ram + ' RAM',
            (m.gpu || []).join(', ') || 'no graphics card reported',
            'display ' + m.display, 'private display: ' + m.virtual_display,
            m.landlock_abi ? 'kernel confinement (Landlock ' + m.landlock_abi + ')' : 'no kernel confinement'];

        return parts.join(' · ');
    }

    function showPolicy(p) {
        policy = p;

        el('res-privacy').value = p.privacy;
        el('res-notify').value = p.notify;
        el('res-auto-models').checked = !!p.autonomy.install_local_models;
        el('res-auto-tools').checked = !!p.autonomy.install_tools;
        el('res-max-download').value = (p.budget.max_download_bytes / gb).toFixed(1);
        el('res-min-free').value = Math.round(p.budget.min_free_disk_bytes / gb);
        el('res-budget').value = p.budget.monthly_api_usd;
        el('res-warn').value = p.thresholds.warning;
        el('res-low').value = p.thresholds.low;
        el('res-critical').value = p.thresholds.critical;
    }

    async function resources(refresh) {
        const list = el('res-list');

        if (!list) return;

        el('res-said').textContent = 'looking…';

        try {
            const body = refresh ? await post('/api/resources/refresh') : await get('/api/resources');

            el('res-machine').textContent = machineLine(body.machine);
            el('res-coding').textContent = 'To write code now:\n' + body.coding.explained;

            list.textContent = '';

            for (const r of body.resources || []) {
                const row = node('div', 'equip-row');

                row.appendChild(node('span', 'equip-title', r.title));
                row.appendChild(node('span', 'note tiny',
                    [r.kind, r.version && 'v' + r.version, stateWord(r), capacity(r),
                        r.plan && 'plan ' + r.plan].filter(Boolean).join(' · ')));
                list.appendChild(row);

                if (r.why) list.appendChild(node('p', 'note tiny equip-under', r.why));
            }

            const runs = el('res-runs');
            runs.textContent = '';

            if ((body.runs || []).length) {
                runs.appendChild(node('p', 'note', 'Recent work:'));

                for (const run of body.runs.slice(0, 10)) {
                    runs.appendChild(node('p', 'note tiny',
                        [run.resource, run.work, run.result, run.failure, run.detail].filter(Boolean).join(' · ')));
                }
            }

            showPolicy(body.policy);
            el('res-said').textContent = '';
        } catch (err) {
            el('res-said').textContent = err.message;
        }
    }

    async function savePolicy() {
        if (!policy) return;

        const p = JSON.parse(JSON.stringify(policy));

        p.privacy = el('res-privacy').value;
        p.notify = el('res-notify').value;
        p.autonomy.install_local_models = el('res-auto-models').checked;
        p.autonomy.install_tools = el('res-auto-tools').checked;
        p.budget.max_download_bytes = Math.round(parseFloat(el('res-max-download').value || '0') * gb);
        p.budget.max_model_bytes = p.budget.max_download_bytes;
        p.budget.min_free_disk_bytes = Math.round(parseFloat(el('res-min-free').value || '0') * gb);
        p.budget.monthly_api_usd = parseFloat(el('res-budget').value || '0');
        p.thresholds.warning = parseFloat(el('res-warn').value);
        p.thresholds.low = parseFloat(el('res-low').value);
        p.thresholds.critical = parseFloat(el('res-critical').value);

        try {
            await post('/api/resources/policy', p);
            el('res-said').textContent = 'Saved. The next piece of work is decided under it.';
            resources(false);
        } catch (err) {
            el('res-said').textContent = err.message;
        }
    }

    function load() {
        packages();
        machine();
        integrations();
        resources(false);
    }

    function wire() {
        document.querySelector('.nav-item[data-view="organisation"]')?.addEventListener('click', load);

        el('proj-go')?.addEventListener('click', () => propose(''));
        el('res-refresh')?.addEventListener('click', () => resources(true));
        el('res-save')?.addEventListener('click', savePolicy);
        el('proj-start')?.addEventListener('click', startProject);
        el('proj-decline')?.addEventListener('click', declineProject);

        for (const id of ['proj-request', 'proj-folder']) {
            el(id)?.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') { e.preventDefault(); propose(''); }
            });
        }

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
