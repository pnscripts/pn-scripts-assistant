/*
 * The recovery key, shown once (ADR 0008).
 *
 * The brain is encrypted with a key this computer keeps. On any other
 * computer it opens only with the recovery key, so it is put in front of its
 * owner the first time, with a way to save it, until he says he has kept it —
 * and then never again.
 */
(async function () {
    const box = document.getElementById('recovery');
    if (!box) return;

    let state;

    try {
        state = await fetch('/api/vault').then((r) => r.json());
    } catch {
        return;
    }

    if (!state || !state.recovery_pending) return;

    let key;

    try {
        key = await fetch('/api/vault/recovery').then((r) => r.json());
    } catch {
        return;
    }

    if (!key || !key.key) return;

    document.getElementById('recovery-key').textContent = key.key;

    if (state.keyring) document.getElementById('recovery-keyring').textContent = state.keyring;
    if (state.warning) document.getElementById('recovery-warning').textContent = state.warning;

    box.hidden = false;

    document.getElementById('recovery-kept').addEventListener('click', async () => {
        await fetch('/api/vault/recovery/kept', { method: 'POST' });
        box.hidden = true;
    });
})();
