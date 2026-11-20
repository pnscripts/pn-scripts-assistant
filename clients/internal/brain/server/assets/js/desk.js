/*
 * desk.js: what this page posts carries the window's key.
 *
 * The assistant's own window is given a key when it opens (the server's
 * desk.go says why): answers on cards and the switches for what other
 * programs may ask are refused from this computer without it, because a
 * program can post to this address as easily as the page can. The key lives
 * in this tab's session storage, which belongs to this address and this tab
 * and is gone when the window closes.
 *
 * Loaded before every other script, so that nothing on the page has to know:
 * each request the page makes to its own address that changes something gets
 * the header. Never a request that only reads, and never one to anywhere else.
 */
(function () {
    'use strict';

    const NAME = 'pn-desk';
    const HEADER = 'X-PN-Desk';

    function kept(storage) {
        try {
            return (storage && storage.getItem(NAME)) || '';
        } catch (err) {
            return '';
        }
    }

    function methodOf(input, init) {
        const named = (init && init.method) || (input && typeof input === 'object' && input.method) || 'GET';

        return String(named).toUpperCase();
    }

    function addressOf(input) {
        if (typeof input === 'string') return input;
        if (input && typeof input === 'object' && typeof input.url === 'string') return input.url;

        return String(input);
    }

    // carry is the whole rule, apart from the page so it can be tested: the
    // options to send with, the key added where it belongs, or `init` as it
    // was given.
    function carry(key, here, input, init) {
        if (!key) return init;

        const method = methodOf(input, init);
        if (method === 'GET' || method === 'HEAD') return init;

        let where;

        try {
            where = new URL(addressOf(input), here);
        } catch (err) {
            return init;
        }

        if (where.origin !== new URL(here).origin) return init;

        const headers = new Headers((init && init.headers) || (input && typeof input === 'object' && input.headers) || undefined);
        headers.set(HEADER, key);

        return Object.assign({}, init, { headers });
    }

    const key = kept(window.sessionStorage);
    const plain = window.fetch.bind(window);

    window.fetch = function (input, init) {
        return plain(input, carry(key, window.location.href, input, init));
    };

    window.pnDesk = {
        carry,
        // Whether this page was opened by the assistant as its own window.
        here: () => key !== '',
        // Whether an answer is the server saying "not from the window".
        refused: (status, body) => status === 403 && Boolean(body) && body.desk === 'not the window',
    };
})();
