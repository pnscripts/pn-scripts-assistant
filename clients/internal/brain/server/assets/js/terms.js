/*
 * The terms of use, readable again on "What it runs on".
 *
 * Accepted once before anything else (agreement.js); here they say when and
 * which version, and show the same words, drawn as text in the page's
 * language and again when it changes.
 */
(function () {
    const card = document.getElementById('terms-card');

    if (!card) return;

    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const lang = () => (window.pnLang ? window.pnLang() : 'en');

    const state = document.getElementById('terms-state');
    const text = document.getElementById('terms-card-text');
    const docs = document.getElementById('terms-docs');

    let terms = null;

    function day(iso) {
        const d = new Date(iso);

        if (Number.isNaN(d.getTime())) return '';

        if (lang() === 'bg') {
            const two = (n) => String(n).padStart(2, '0');

            return `${two(d.getDate())}.${two(d.getMonth() + 1)}.${d.getFullYear()}`;
        }

        return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' });
    }

    function draw() {
        if (!terms) return;

        const shown = lang() === 'bg' ? terms.bg : terms.en;

        state.textContent = terms.record
            ? T('terms.accepted', { date: day(terms.record.accepted_at), version: terms.record.version })
            : shown.version;

        text.replaceChildren();

        for (const part of shown.sections) {
            const heading = document.createElement('h3');
            heading.textContent = part.heading;

            const body = document.createElement('p');
            body.textContent = part.body;

            text.append(heading, body);
        }
    }

    docs.addEventListener('click', () => {
        if (!terms || !terms.online) return;

        fetch('/api/open-source', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ where: lang() === 'bg' ? terms.online.bg : terms.online.en }),
        }).catch(() => {});
    });

    fetch('/api/terms', { headers: { Accept: 'application/json' } })
        .then((res) => (res.ok ? res.json() : null))
        .then((body) => {
            terms = body;
            draw();
        })
        .catch(() => {});

    window.addEventListener('pn-language', draw);
})();
