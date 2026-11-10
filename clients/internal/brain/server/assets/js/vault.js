/*
 * The recovery key, shown once (ADR 0008).
 *
 * The brain is encrypted with a key this computer keeps. On any other
 * computer it opens only with the recovery key, so it is put in front of its
 * owner the first time, with a way to save it, until he says he has kept it,
 * and then never again.
 *
 * Where the key is kept is said by the server as a kind (keyring_key) and,
 * for a key file, its folder, so the notice says it in the page's language.
 */
(async function () {
    const box = document.getElementById('recovery');
    if (!box) return;

    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

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

    function draw() {
        const keyring = state.keyring_key
            ? T(state.keyring_key, { dir: state.keyring_dir || '' })
            : (state.keyring || T('vault.keyring.unknown'));

        document.getElementById('recovery-note').textContent = T('vault.note', { keyring });
        document.getElementById('recovery-warning').textContent = state.warning_key
            ? T(state.warning_key)
            : (state.warning || '');
    }

    draw();
    window.addEventListener('pn-language', draw);

    box.hidden = false;

    document.getElementById('recovery-kept').addEventListener('click', async () => {
        await fetch('/api/vault/recovery/kept', { method: 'POST' });
        box.hidden = true;
    });
})();
