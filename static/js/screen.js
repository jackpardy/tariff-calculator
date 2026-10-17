// screen.js: the venue screen (views/screen.templ). Fetches the page again
// every 30 seconds and swaps in what it shows, keeping the last one that
// loaded when the connection drops, and says so once it's more than a minute
// old. The clock ticks on from the server's time, in the competition's time
// zone, whatever the screen's own clock says. Announcements take turns, one
// at a time, every 8 seconds. The cards' text shrinks until
// everything fits, down to a size readable across a hall, then shows fewer
// names, so no name is ever cut off unseen.
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
    // so they look alike. It stops at a size still readable across a hall;
    // past that, a card shows fewer names and counts the rest in "+N more".
    const floor = () => 0.85 * parseFloat(getComputedStyle(document.documentElement).fontSize);
    const overflows = (part) => part.scrollHeight > part.clientHeight + 1;

    function fit() {
        const cards = [...main.querySelectorAll('.screen-card')];
        if (cards.length === 0) { return; }
        untrim();
        cards.forEach((c) => { c.style.fontSize = ''; });
        let size = parseFloat(getComputedStyle(cards[0]).fontSize);
        const over = () => cards.some((c) => [...c.querySelectorAll('.screen-part')].some(overflows));
        while (over() && size * 0.95 >= floor()) {
            size *= 0.95;
            cards.forEach((c) => { c.style.fontSize = size + 'px'; });
        }
        cards.forEach(trim);
    }

    // untrim shows again the names an earlier fit hid, with the "+N more"
    // as the server sent it, so a fit with more room (an announcement gone)
    // starts afresh.
    function untrim() {
        for (const list of main.querySelectorAll('.screen-names')) {
            list.querySelectorAll('li[hidden]').forEach((li) => { li.hidden = false; });
            const more = list.querySelector('.screen-more');
            if (!more) { continue; }
            if (more.dataset.added) { more.remove(); continue; }
            more.dataset.base ??= more.dataset.more;
            more.dataset.more = more.dataset.base;
            more.textContent = `+${more.dataset.base} more`;
        }
    }

    // trim hides names from the end of an overflowing part's list until it
    // fits, adding them to its "+N more".
    function trim(card) {
        for (const part of card.querySelectorAll('.screen-part')) {
            const list = part.querySelector('.screen-names');
            if (!list) { continue; }
            let more = list.querySelector('.screen-more');
            while (overflows(part)) {
                const shown = [...list.querySelectorAll('li:not(.screen-more):not([hidden])')];
                if (shown.length <= 1) { break; }
                shown[shown.length - 1].hidden = true;
                if (!more) {
                    more = document.createElement('li');
                    more.className = 'screen-more';
                    more.dataset.more = '0';
                    more.dataset.added = '1';
                    list.append(more);
                }
                more.dataset.more = String(Number(more.dataset.more) + 1);
                more.textContent = `+${more.dataset.more} more`;
            }
        }
    }

    // Announcements take turns: the one shown changes every noticeEvery,
    // and carries on in turn when the page is fetched again.
    const noticeEvery = 8 * 1000;
    let noticeAt = 0;
    function showNotice() {
        const notices = [...main.querySelectorAll('[data-notice]')];
        if (notices.length === 0) { return; }
        noticeAt %= notices.length;
        notices.forEach((n, i) => { n.hidden = i !== noticeAt; });
    }
    // Each announcement takes its own room: the cards fit again around it,
    // or the last names are cut by the card's edge.
    function nextNotice() {
        noticeAt++;
        showNotice();
        fit();
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
            showNotice();
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

    showNotice();
    fit();
    window.addEventListener('resize', fit);
    tick();
    setInterval(tick, 1000);
    setInterval(refresh, every);
    setInterval(nextNotice, noticeEvery);
})();
