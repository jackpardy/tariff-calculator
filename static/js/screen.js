// screen.js: the venue screen (views/screen.templ). Fetches the page again
// every 30 seconds and swaps in what it shows, keeping the last one that
// loaded when the connection drops, and says so once it's more than a minute
// old. The clock ticks on from the server's time, in the competition's time
// zone, whatever the screen's own clock says. The cards' text shrinks until
// everything fits, so no name is ever cut off unseen.
(function () {
    const every = 30 * 1000;
    const staleAfter = 75 * 1000;

    let main = document.querySelector('[data-screen]');
    if (!main) { return; }
    let lastLoaded = Date.now();
    let offset = Number(main.dataset.at) - Date.now(); // server clock less this one
    const zone = main.dataset.zone || undefined;
    const format = (ms, seconds) => new Intl.DateTimeFormat('en-GB', {
        hour: '2-digit', minute: '2-digit', second: seconds ? '2-digit' : undefined, hour12: false, timeZone: zone,
    }).format(new Date(ms));

    function tick() {
        const now = Date.now();
        const clock = main.querySelector('[data-clock]');
        if (clock) { clock.textContent = format(now + offset, false); }
        const stale = main.querySelector('[data-stale]');
        if (stale) {
            stale.hidden = now - lastLoaded < staleAfter;
            const since = stale.querySelector('[data-stale-since]');
            if (since) { since.textContent = format(lastLoaded + offset, false); }
        }
    }

    // fit shrinks the cards' text, a step at a time, until none of their
    // parts overflows, and gives every card the size the fullest one needed,
    // so they look alike.
    function fit() {
        const cards = [...main.querySelectorAll('.screen-card')];
        if (cards.length === 0) { return; }
        cards.forEach((c) => { c.style.fontSize = ''; });
        let size = parseFloat(getComputedStyle(cards[0]).fontSize);
        const over = () => cards.some((c) => [...c.querySelectorAll('.screen-part')].some((p) => p.scrollHeight > p.clientHeight + 1));
        while (over() && size > 10) {
            size *= 0.95;
            cards.forEach((c) => { c.style.fontSize = size + 'px'; });
        }
    }

    async function refresh() {
        try {
            const response = await fetch(location.href, { cache: 'no-store' });
            if (!response.ok) { return; }
            const page = new DOMParser().parseFromString(await response.text(), 'text/html');
            const fresh = page.querySelector('[data-screen]');
            if (!fresh) { return; }
            main.replaceWith(fresh);
            main = fresh;
            fit();
            lastLoaded = Date.now();
            offset = Number(main.dataset.at) - lastLoaded;
            // A new version of the app reloads the whole page, for its new styles.
            if (page.querySelector('link[rel=stylesheet][href*="styles"]')?.getAttribute('href')
                !== document.querySelector('link[rel=stylesheet][href*="styles"]')?.getAttribute('href')) {
                location.reload();
                return;
            }
        } catch (e) {
            // Offline or the server didn't answer: keep what's shown; tick says how old it is.
        } finally {
            tick();
        }
    }

    fit();
    window.addEventListener('resize', fit);
    tick();
    setInterval(tick, 1000);
    setInterval(refresh, every);
})();
