/*
 * Who it works for, and what it has been taught.
 *
 * Both are files somebody wrote, and both can be edited here or in an editor —
 * so this reads the file rather than holding a copy, and never overwrites a
 * box somebody is typing in. The last time a panel here rebuilt itself on a
 * timer while somebody was typing, it threw away half a sentence every two
 * seconds and looked like a broken keyboard.
 */
(function () {
    'use strict';

    const el = (id) => document.getElementById(id);

    // Whether either form is being edited. Nothing reloads over a cursor.
    let editingProfile = false;
    let editingSkill = false;

    function say(id, text) {
        const node = el(id);
        if (node) node.textContent = text || '';
    }

    /* ---------- about you ---------- */

    function countProfile() {
        const box = el('profile-text');
        const most = Number(box?.dataset.most || 0);
        if (!box || !most) return;

        // Counted in letters rather than bytes: Cyrillic is two bytes a
        // letter, and a limit measured in bytes would cut a Bulgarian profile
        // in half without explaining itself.
        const used = [...box.value].length;

        say('profile-count', used + ' of ' + most + ' letters'
            + (used > most ? ' — the rest will not be kept' : ''));
    }

    async function loadProfile() {
        if (editingProfile) return;

        let body;

        try {
            const res = await fetch('/api/profile');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const box = el('profile-text');
        if (box) {
            box.value = body.profile || '';
            box.dataset.most = body.most || 0;
        }

        countProfile();

        say('profile-where', body.to_hosted
            ? 'This is given to whichever model answers, including paid ones. '
              + 'Change that in Privacy.'
            : 'This is only ever given to the model on this machine. Paid services '
              + 'are not told any of it. Change that in Privacy.');
    }

    async function saveProfile() {
        const box = el('profile-text');
        if (!box) return;

        say('profile-said', 'Saving…');

        try {
            const res = await fetch('/api/profile', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ profile: box.value })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('profile-said', body.error || 'That did not save.');
                return;
            }

            // Shown as it was kept, not as it was typed — so somebody who went
            // over the ceiling sees what actually survived.
            editingProfile = false;
            box.value = body.profile || '';
            countProfile();
            say('profile-said', 'Saved. It is read again on your next message.');
        } catch (err) {
            say('profile-said', 'That did not save: ' + err.message);
        }
    }

    /* ---------- what it has been taught ---------- */

    function skillRow(skill) {
        const row = document.createElement('div');
        row.className = 'task-row';

        const name = document.createElement('span');
        name.className = 'skill-name';
        name.textContent = skill.name;
        row.appendChild(name);

        const when = document.createElement('span');
        when.className = 'task-state';
        when.textContent = skill.when || 'no note on when to use it';
        row.appendChild(when);

        // Where it came from, because a skill it wrote itself and one somebody
        // typed deserve different amounts of trust when something goes wrong.
        if (skill.taught) {
            const how = document.createElement('span');
            how.className = 'skill-origin';
            how.textContent = 'taught';
            row.appendChild(how);
        }

        const read = document.createElement('button');
        read.type = 'button';
        read.className = 'task-button';
        read.textContent = 'Read';
        read.addEventListener('click', () => showSkill(skill));
        row.appendChild(read);

        const forget = document.createElement('button');
        forget.type = 'button';
        forget.className = 'task-button';
        forget.textContent = 'Forget';
        forget.addEventListener('click', () => forgetSkill(skill.name));
        row.appendChild(forget);

        return row;
    }

    function showSkill(skill) {
        const list = el('skill-list');
        if (!list) return;

        let shown = document.getElementById('skill-shown');

        if (shown) shown.remove();

        shown = document.createElement('pre');
        shown.id = 'skill-shown';
        shown.className = 'task-evidence';
        shown.textContent = skill.how;
        list.appendChild(shown);
    }

    async function forgetSkill(name) {
        say('skill-said', 'Forgetting ' + name + '…');

        try {
            const res = await fetch('/api/skills/' + encodeURIComponent(name), {
                method: 'DELETE'
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('skill-said', body.error || 'That did not work.');
                return;
            }

            say('skill-said', 'Forgotten. It is no longer offered.');
            loadSkills();
        } catch (err) {
            say('skill-said', 'That did not work: ' + err.message);
        }
    }

    async function saveSkill() {
        const name = el('skill-name')?.value.trim();
        const when = el('skill-when')?.value.trim();
        const how = el('skill-how')?.value.trim();

        if (!name || !how) {
            say('skill-said', 'It needs a name and some instructions.');
            return;
        }

        say('skill-said', 'Saving…');

        try {
            const res = await fetch('/api/skills', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: name, when: when, how: how })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say('skill-said', body.error || 'That did not save.');
                return;
            }

            el('skill-name').value = '';
            el('skill-when').value = '';
            el('skill-how').value = '';
            editingSkill = false;

            const details = el('skill-new');
            if (details) details.open = false;

            say('skill-said', 'Saved. It can use it from your next message.');
            loadSkills();
        } catch (err) {
            say('skill-said', 'That did not save: ' + err.message);
        }
    }

    async function loadSkills() {
        if (editingSkill) return;

        let body;

        try {
            const res = await fetch('/api/skills');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const list = el('skill-list');
        if (!list) return;

        list.textContent = '';

        const found = body.skills || [];

        if (!found.length) {
            const p = document.createElement('p');
            p.className = 'note';
            p.textContent = 'Nothing yet. Tell it "remember how I do this" in a '
                + 'conversation, or write one below.';
            list.appendChild(p);
        } else {
            for (const skill of found) list.appendChild(skillRow(skill));
        }

        say('skill-folder', 'They are files in ' + (body.folder || '')
            + ' — you can edit them there too.');
    }

    function wire() {
        el('profile-save')?.addEventListener('click', saveProfile);

        const box = el('profile-text');
        if (box) {
            box.addEventListener('input', () => { editingProfile = true; countProfile(); });
            box.addEventListener('blur', () => { editingProfile = false; });
        }

        el('skill-save')?.addEventListener('click', saveSkill);

        for (const id of ['skill-name', 'skill-when', 'skill-how']) {
            el(id)?.addEventListener('input', () => { editingSkill = true; });
        }

        loadProfile();
        loadSkills();

        /*
         * Reloaded when the view is opened, not on a timer.
         *
         * Nothing here changes on its own except a skill the assistant wrote
         * during a conversation, and coming back to the panel is exactly when
         * somebody wants to see that. A five-second poll over two text boxes
         * would only ever be a way to lose what somebody was typing.
         */
        document.querySelector('.nav-item[data-view="teaching"]')
            ?.addEventListener('click', () => { loadProfile(); loadSkills(); });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
