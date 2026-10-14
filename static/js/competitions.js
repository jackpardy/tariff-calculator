// competitions.js: the competition and club pages (views/competitions.templ,
// views/clubs.templ). Copies links; keeps organisers', comp secs', members'
// and gymnasts' links in this browser and lists them; offers the levels saved
// here when creating a competition, and the clubs saved here when entering
// one; fills each voluntary on an entry form from the routines saved here;
// remembers which levels of the organiser's dashboard are open; updates the
// On the day page in place as its buttons are tapped; and registers the
// service worker that keeps pages for offline, with a banner when offline.
(function () {
    // Links saved in this browser: {name, url} lists under these keys.
    const linkKeys = {
        competitions: 'trampolineCompetitions',
        clubs: 'trampolineClubs',
        members: 'trampolineMemberLinks',
        coaches: 'trampolineCoachLinks',
        entries: 'trampolineEntryLinks',
    };

    function loadLinks(key) {
        try {
            const links = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(links) ? links.filter((l) => l && l.url) : [];
        } catch (e) {
            return [];
        }
    }

    // keepLink saves a link, replacing one with the same name (a replaced admin
    // link) or URL.
    function keepLink(key, name, url) {
        const links = loadLinks(key).filter((l) => l.url !== url && l.name !== name);
        links.unshift({ name, url });
        try { localStorage.setItem(key, JSON.stringify(links)); } catch (e) { /* not saved: the page says to copy it */ }
    }

    function copyButtons() {
        for (const button of document.querySelectorAll('[data-copy]')) {
            button.addEventListener('click', async () => {
                try {
                    await navigator.clipboard.writeText(button.dataset.copy);
                    button.textContent = 'Copied';
                } catch (e) {
                    button.closest('.comp-link')?.querySelector('input')?.select();
                }
            });
        }
    }

    function saveLinks() {
        const admin = document.querySelector('[data-competition-admin]');
        if (admin) { keepLink(linkKeys.competitions, admin.dataset.competitionName, admin.dataset.competitionAdmin); }
        const entry = document.querySelector('[data-entry-link]');
        if (entry) { keepLink(linkKeys.entries, entry.dataset.entryName, entry.dataset.entryLink); }
        const club = document.querySelector('[data-club-admin]');
        if (club) { keepLink(linkKeys.clubs, club.dataset.clubName, club.dataset.clubAdmin); }
        const member = document.querySelector('[data-member-link]');
        if (member) { keepLink(linkKeys.members, member.dataset.memberName, member.dataset.memberLink); }
        const coach = document.querySelector('[data-coach-link]');
        if (coach) { keepLink(linkKeys.coaches, coach.dataset.coachName, coach.dataset.coachLink); }
    }

    // linkItem is a list item linking to a saved link.
    function linkItem(link) {
        const a = Object.assign(document.createElement('a'), { href: link.url, textContent: link.name });
        const item = document.createElement('li');
        item.append(a);
        return item;
    }

    // hub lists every link saved in this browser.
    function hub() {
        let any = false;
        for (const box of document.querySelectorAll('[data-hub-list]')) {
            const links = loadLinks(box.dataset.hubList);
            box.querySelector('ul').append(...links.map(linkItem));
            box.hidden = links.length === 0;
            any = any || links.length > 0;
        }
        const empty = document.querySelector('[data-hub-empty]');
        if (empty) { empty.hidden = any; }
    }

    // clubLink offers the clubs saved in this browser for entering a competition.
    function clubLink() {
        const field = document.querySelector('[data-saved-clubs]');
        const input = document.getElementById('club-admin');
        const clubs = loadLinks(linkKeys.clubs);
        if (!field || !input || clubs.length === 0) { return; }
        const select = field.querySelector('select');
        for (const c of clubs) { select.append(new Option(c.name, c.url)); }
        select.append(new Option('Another club (paste its admin link)', ''));
        const choose = () => {
            input.value = select.value;
            input.closest('.field').hidden = select.value !== '';
        };
        select.addEventListener('change', choose);
        field.hidden = false;
        choose();
    }

    // newCompetition lists this browser's competitions and offers its own levels,
    // each posted as {level, sets}: the level and copies of the requirements it
    // names, which the server keeps with the competition.
    function newCompetition() {
        const saved = document.getElementById('saved-competitions');
        const competitions = loadLinks(linkKeys.competitions);
        if (saved && competitions.length > 0) {
            saved.querySelector('ul').append(...competitions.map(linkItem));
            saved.hidden = false;
        }

        const own = document.getElementById('own-levels');
        if (!own || typeof LevelStore === 'undefined') { return; }
        const sets = SetStore.load();
        for (const { level } of LevelStore.load()) {
            const named = {};
            for (const n of [1, 2]) {
                for (const ref of ((n === 2 && level.second) || level.first)?.options || []) {
                    const set = sets.find((s) => s.id === ref);
                    if (set) { named[ref] = set.set; }
                }
            }
            const label = document.createElement('label');
            label.className = 'checkbox comp-level';
            const input = Object.assign(document.createElement('input'), { type: 'checkbox', name: 'custom', value: JSON.stringify({ level, sets: named }) });
            label.append(input, ' ' + level.name);
            own.append(label);
        }
        own.hidden = own.querySelectorAll('input').length === 0;
    }

    // entryForms show the chosen level's exercises, and for each voluntary a
    // choice of the routines saved in this browser, whose skills are posted.
    function entryForms() {
        for (const form of document.querySelectorAll('.comp-entry-form')) {
            const levelSelect = form.querySelector('select[name="level"]');
            const routines = typeof RoutineStore === 'undefined' ? [] : RoutineStore.load().routines.filter((r) => r.skills.length > 0);

            const showLevel = () => {
                for (const fieldset of form.querySelectorAll('.comp-entry-level')) {
                    const on = fieldset.dataset.level === levelSelect.value;
                    fieldset.hidden = !on;
                    fieldset.disabled = !on;
                }
            };

            for (const box of form.querySelectorAll('.comp-entry-exercise')) {
                const skills = box.querySelector(`input[name="${box.dataset.exercise}Skills"]`);
                const select = box.querySelector('select[data-routine-for]');
                if (!select.querySelector('option[value="current"]')) {
                    select.append(new Option(routines.length > 0 ? 'Choose a routine' : 'No routines saved in this browser', ''));
                }
                for (const r of routines) {
                    select.append(new Option(`${r.name} (${r.skills.length} skills)`, r.id));
                }
                select.addEventListener('change', () => {
                    if (select.value === 'current') { return; }
                    const routine = routines.find((r) => r.id === select.value);
                    skills.value = routine ? JSON.stringify(routine.skills) : '';
                });
                // A set routine is performed as written: no routine to choose.
                const showRoutine = () => {
                    const option = box.querySelector('input[type="radio"]:checked, input[type="hidden"][data-set-routine]');
                    const setRoutine = option?.dataset.setRoutine === 'true';
                    box.querySelector('.comp-routine').hidden = setRoutine || !option;
                    box.querySelector('.comp-set-note').hidden = !setRoutine;
                };
                box.querySelectorAll('input[type="radio"]').forEach((radio) => radio.addEventListener('change', showRoutine));
                showRoutine();
            }
            levelSelect.addEventListener('change', showLevel);
            showLevel();
        }
    }

    // partnerLink offers the member pages and entries saved in this browser for
    // a synchro partner to say who they are.
    function partnerLink() {
        const field = document.querySelector('[data-saved-people]');
        const input = document.getElementById('partner-link');
        const people = [...loadLinks(linkKeys.members), ...loadLinks(linkKeys.entries)];
        if (!field || !input || people.length === 0) { return; }
        const select = field.querySelector('select');
        for (const p of people) { select.append(new Option(p.name, p.url)); }
        select.append(new Option('Someone else, or no other entry', ''));
        const choose = () => {
            input.value = select.value;
            input.closest('.field').hidden = select.value !== '';
        };
        select.addEventListener('change', choose);
        field.hidden = false;
        choose();
    }

    // videoChoices select "Video of some skills" when one of its skills is chosen.
    function videoChoices() {
        for (const fieldset of document.querySelectorAll('.comp-video')) {
            const some = fieldset.querySelector('input[name="video"][value="skills"]');
            fieldset.querySelectorAll('.comp-video-skills input').forEach((input) => input.addEventListener('input', () => {
                if (input.type !== 'checkbox' || input.checked) { some.checked = true; }
            }));
        }
    }

    // openLevels remembers, in this browser, which levels of an organiser's
    // dashboard are open, and opens them again next time. A search or filter
    // opens the levels it finds people in instead, so it isn't remembered.
    function openLevels() {
        const levels = document.querySelectorAll('details.comp-dash-level[data-remember]');
        if (levels.length === 0) {
            return;
        }
        const key = 'trampolineOpenLevels:' + location.pathname;
        let open = [];
        try { open = JSON.parse(localStorage.getItem(key) || '[]'); } catch (e) { /* none remembered */ }
        if (!Array.isArray(open)) {
            open = [];
        }
        levels.forEach((d) => { if (open.includes(d.dataset.level)) { d.open = true; } });
        const save = () => {
            const now = Array.from(levels).filter((d) => d.open).map((d) => d.dataset.level);
            try { localStorage.setItem(key, JSON.stringify(now)); } catch (e) { /* not remembered */ }
        };
        levels.forEach((d) => d.addEventListener('toggle', save));
    }

    // menuInView scrolls each row of the page's menu sideways so its current
    // tab shows: on a phone the rows are wider than the screen.
    function menuInView() {
        document.querySelectorAll('.comp-menu .tabs').forEach((row) => {
            const active = row.querySelector('li.is-active');
            if (active && row.scrollWidth > row.clientWidth) {
                const a = active.getBoundingClientRect();
                const r = row.getBoundingClientRect();
                row.scrollLeft += a.left - r.left - (r.width - a.width) / 2;
            }
        });
    }

    // dayPage keeps the On the day page where it is when a button on it is
    // tapped (Here, Scratched, Started, Notify...). The form is sent in the
    // background, then the notice, the scratch warnings and the areas are
    // swapped for fresh ones, with the lists that were open still open and the
    // page not moving. Which lists are open is also kept for this tab, so a
    // reload keeps them. If anything goes wrong the form is posted the usual
    // way, as it is without JavaScript, and the server sends the browser back
    // to the flight's box.
    function dayPage() {
        if (!document.getElementById('day-areas')?.hasAttribute('data-day-page')) {
            return;
        }
        const key = 'trampolineOpenDay:' + location.pathname + '?day=' + (new URLSearchParams(location.search).get('day') || '');
        const openKeys = (root) => new Set(Array.from(root.querySelectorAll('details[data-key][open]'), (d) => d.dataset.key));
        const reopen = (root, keys) => root.querySelectorAll('details[data-key]').forEach((d) => { d.open = keys.has(d.dataset.key); });
        const save = () => {
            try { sessionStorage.setItem(key, JSON.stringify(Array.from(openKeys(document)))); } catch (e) { /* not remembered */ }
        };

        try {
            const kept = JSON.parse(sessionStorage.getItem(key) || '[]');
            if (Array.isArray(kept)) { reopen(document, new Set(kept)); }
        } catch (e) { /* none remembered */ }
        // toggle doesn't bubble, so listen on the way down.
        document.addEventListener('toggle', (event) => {
            if (event.target.matches?.('details[data-key]')) { save(); }
        }, true);

        // swap puts a fresh copy of the page's changing parts in place of the
        // current ones. If the areas on the page changed, all of them go.
        const swap = (fresh) => {
            for (const id of ['day-notice', 'day-warnings']) {
                const now = document.getElementById(id);
                const next = fresh.getElementById(id);
                if (now && next) { now.replaceWith(document.importNode(next, true)); }
            }
            const now = document.getElementById('day-areas');
            const next = fresh.getElementById('day-areas');
            const ids = (root) => Array.from(root.querySelectorAll('[id^="area-"]'), (a) => a.id);
            if (ids(now).join() !== ids(next).join()) {
                now.replaceWith(document.importNode(next, true));
                return;
            }
            for (const id of ids(next)) {
                document.getElementById(id).replaceWith(document.importNode(fresh.getElementById(id), true));
            }
        };

        // plainSubmit posts the form the usual way, with the button that was
        // tapped (form.submit() leaves it out).
        const plainSubmit = (form, submitter) => {
            if (!form.isConnected) {
                location.reload();
                return;
            }
            if (submitter) {
                submitter.disabled = false;
                if (submitter.name) {
                    form.append(Object.assign(document.createElement('input'), { type: 'hidden', name: submitter.name, value: submitter.value }));
                }
            }
            form.submit();
        };

        // send posts the form, and swaps in the page it ends up at; shut is
        // the data-key of a list to leave closed.
        const send = async (form, submitter, body, shut) => {
            let fresh;
            try {
                const response = await fetch(form.action, { method: 'POST', body, credentials: 'same-origin' });
                if (!response.ok) { throw new Error('status ' + response.status); }
                fresh = new DOMParser().parseFromString(await response.text(), 'text/html');
                if (!fresh.getElementById('day-areas')?.hasAttribute('data-day-page')) { throw new Error('not the day page'); }
            } catch (e) {
                plainSubmit(form, submitter);
                return;
            }
            try {
                // Keep the flight where it is on the screen, whatever changes above it.
                const anchor = form.closest('.comp-flight')?.id || 'day-areas';
                const before = document.getElementById(anchor).getBoundingClientRect().top;
                const keys = openKeys(document);
                keys.delete(shut);
                reopen(fresh, keys);
                swap(fresh);
                const after = document.getElementById(anchor)?.getBoundingClientRect().top;
                if (after !== undefined && after !== before) { window.scrollBy(0, after - before); }
                save();
                // A notice left in the address would come back on a reload.
                const url = new URL(location.href);
                if (url.searchParams.has('notice')) {
                    url.searchParams.delete('notice');
                    history.replaceState(null, '', url);
                }
            } catch (e) {
                location.reload();
            }
        };

        // One at a time, so a slow answer can't undo a newer one.
        let queue = Promise.resolve();
        document.addEventListener('submit', (event) => {
            const form = event.target;
            if (event.defaultPrevented || !form.closest('#day-areas, #day-warnings')) {
                return;
            }
            event.preventDefault();
            const submitter = event.submitter;
            // Sent as a plain form (not multipart), as the server expects, with
            // the button that was tapped.
            const body = new URLSearchParams(new FormData(form, submitter));
            if (submitter) { submitter.disabled = true; }
            // A Notify list closes once it has been sent.
            const shut = form.closest('details.comp-notify')?.dataset.key;
            queue = queue.then(() => send(form, submitter, body, shut));
        });
    }

    // offlineBanner shows the "you're offline" banner while the phone has no
    // connection, saying when this page was loaded (the page-time meta, set
    // by the server when it rendered the page, so a copy the service worker
    // kept says when it was last opened).
    function offlineBanner() {
        const banner = document.querySelector('.comp-offline');
        if (!banner) { return; }
        const when = document.querySelector('meta[name="page-time"]')?.content || 'at an unknown time';
        for (const el of banner.querySelectorAll('[data-offline-when]')) { el.textContent = when; }
        // A copy the service worker served because the network was too slow
        // is marked, so the banner shows though the phone counts as online.
        const fromCopy = banner.hasAttribute('data-from-copy');
        const show = () => { banner.hidden = navigator.onLine && !fromCopy; };
        window.addEventListener('offline', show);
        window.addEventListener('online', show);
        show();
    }

    // keepOffline registers the service worker (sw.js), which keeps the pages
    // opened on this phone so they still open without a connection.
    function keepOffline() {
        if ('serviceWorker' in navigator) {
            navigator.serviceWorker.register('/sw.js').catch(() => { /* no offline copies */ });
        }
    }

    document.addEventListener('DOMContentLoaded', () => {
        keepOffline();
        offlineBanner();
        openLevels();
        menuInView();
        dayPage();
        copyButtons();
        saveLinks();
        newCompetition();
        hub();
        clubLink();
        partnerLink();
        videoChoices();
        entryForms();
    });
})();
