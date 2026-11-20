/*
 * desk-enter.js: the page behind the window's first address.
 *
 * The address carries a code that works once. It is swapped here for the key
 * this window keeps for as long as it is open (desk.js), and the address is
 * replaced, so the code is not left where "back" would find it.
 */
(async function () {
    'use strict';

    const code = window.location.pathname.split('/').pop() || '';

    try {
        const res = await fetch('/api/desk/enter', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: JSON.stringify({ code }),
        });

        const body = await res.json();

        if (res.ok && body && body.key) {
            window.sessionStorage.setItem('pn-desk', body.key);
            window.location.replace('/');

            return;
        }
    } catch (err) {
        /* said below, the same way whatever went wrong */
    }

    document.getElementById('desk-opening').hidden = true;
    document.getElementById('desk-failed').hidden = false;
})();
