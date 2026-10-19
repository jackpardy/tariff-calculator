// competitions.js: the competition and club pages (views/competitions.templ,
// views/clubs.templ). Copies links; keeps organisers', comp secs', members'
// and gymnasts' links in this browser and lists them; offers the levels saved
// here when creating a competition, and the clubs saved here when entering
// one; fills each voluntary on an entry form from the routines saved here;
// remembers which levels of the organiser's dashboard are open; updates the
// On the day page in place as its buttons are tapped, keeping taps made
// without signal on the phone until they can be sent (taps.js); and registers the
// service worker that keeps pages for offline, with a banner when offline.
(function () {
    // Links saved in this browser: {name, url} lists under these keys.
    const linkKeys = {
        competitions: 'trampolineCompetitions',
        clubs: 'trampolineClubs',
        members: 'trampolineMemberLinks',
        coaches: 'trampolineCoachLinks',
        gymnasts: 'trampolineGymnastLinks',
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

    // keepLink saves a link, replacing one with the same URL, with anything
    // more to know about it (extra: e.g. that a member coaches too). A
    // replaced admin link, or a personal link (named for the person and the
    // club or competition), takes the place of the one with its name
    // (replaces); otherwise two competitions or clubs of one name are both kept.
    function keepLink(key, name, url, extra = {}, replaces = false) {
        const links = loadLinks(key).filter((l) => l.url !== url && !(replaces && l.name === name));
        links.unshift({ name, url, ...extra });
        try { localStorage.setItem(key, JSON.stringify(links)); } catch (e) { /* not saved: the page says to copy it */ }
    }

    // forgetDeadLink drops, from every list of links saved in this browser,
    // the one this page found leads nowhere: the link itself, or a
    // competition's admin link just deleted (".../delete").
    function forgetDeadLink() {
        if (!document.querySelector('[data-dead-link]')) { return; }
        const here = location.pathname.replace(/\/delete$/, '');
        for (const key of Object.values(linkKeys)) {
            const links = loadLinks(key);
            const kept = links.filter((l) => {
                try { return new URL(l.url, location.href).pathname !== here; } catch (e) { return true; }
            });
            if (kept.length !== links.length) {
                try { localStorage.setItem(key, JSON.stringify(kept)); } catch (e) { /* nothing saved to change */ }
            }
        }
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
        if (admin) { keepLink(linkKeys.competitions, admin.dataset.competitionName, admin.dataset.competitionAdmin, {}, admin.hasAttribute('data-replaces')); }
        const entry = document.querySelector('[data-entry-link]');
        if (entry) { keepLink(linkKeys.entries, entry.dataset.entryName, entry.dataset.entryLink, {}, true); }
        const club = document.querySelector('[data-club-admin]');
        if (club) { keepLink(linkKeys.clubs, club.dataset.clubName, club.dataset.clubAdmin, {}, club.hasAttribute('data-replaces')); }
        const member = document.querySelector('[data-member-link]');
        if (member) { keepLink(linkKeys.members, member.dataset.memberName, member.dataset.memberLink, { coach: member.hasAttribute('data-member-coach') }, true); }
        const coach = document.querySelector('[data-coach-link]');
        if (coach) { keepLink(linkKeys.coaches, coach.dataset.coachName, coach.dataset.coachLink, {}, true); }
        const gymnast = document.querySelector('[data-gymnast-link]');
        if (gymnast) { keepLink(linkKeys.gymnasts, gymnast.dataset.gymnastName, gymnast.dataset.gymnastLink, {}, true); }
        // An organiser's or comp sec's admin page saves its link whenever it's
        // opened, as the other pages do; never a helper's link or a page opened
        // through someone's own link (a "~" in the path), which isn't theirs to keep.
        if (location.pathname.includes('~')) { return; }
        const competition = document.querySelector('[data-competition-page]');
        if (competition) { keepLink(linkKeys.competitions, competition.dataset.competitionName, location.origin + competition.dataset.competitionPage); }
        const clubPage = document.querySelector('[data-club-page]');
        if (clubPage) { keepLink(linkKeys.clubs, clubPage.dataset.clubName, location.origin + clubPage.dataset.clubPage); }
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
    // choice of the routines kept on the member's page (from the server) and
    // those saved in this browser, whose skills are posted.
    function entryForms() {
        for (const form of document.querySelectorAll('.comp-entry-form')) {
            const levelSelect = form.querySelector('select[name="level"]');
            const routines = typeof RoutineStore === 'undefined' ? [] : RoutineStore.load().routines.filter((r) => r.skills.length > 0);

            const category = form.querySelector('.comp-entry-category');
            const showLevel = () => {
                let split = false;
                for (const fieldset of form.querySelectorAll('.comp-entry-level')) {
                    const on = fieldset.dataset.level === levelSelect.value;
                    fieldset.hidden = !on;
                    fieldset.disabled = !on;
                    if (on && fieldset.dataset.split === 'true') { split = true; }
                }
                // One Men or Women choice, kept as the level changes.
                category.hidden = !split;
                category.disabled = !split;
                category.querySelector('[data-category-level]').textContent = levelSelect.value;
            };

            for (const box of form.querySelectorAll('.comp-entry-exercise')) {
                const skills = box.querySelector(`input[name="${box.dataset.exercise}Skills"]`);
                const select = box.querySelector('select[data-routine-for]');
                const kept = select.querySelector('optgroup[data-kept]');
                if (!select.querySelector('option[value="current"]')) {
                    const any = routines.length > 0 || kept;
                    const prompt = new Option(any ? 'Choose a routine' : 'No routines saved in this browser', '');
                    select.prepend(prompt);
                    prompt.selected = true;
                }
                if (routines.length > 0) {
                    const group = kept ? Object.assign(document.createElement('optgroup'), { label: 'Saved in this browser' }) : select;
                    for (const r of routines) {
                        group.append(new Option(`${r.name} (${r.skills.length} skills)`, r.id));
                    }
                    if (kept) { select.append(group); }
                }
                select.addEventListener('change', () => {
                    if (select.value === 'current') { return; }
                    const option = select.selectedOptions[0];
                    if (option?.dataset.skills) { skills.value = option.dataset.skills; return; }
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

    // savedPeople offers the people's pages saved in this browser in a field's
    // select, filling in the input with the one chosen, or showing it to paste
    // another when the last option (other) is. With a prompt, the select starts
    // on it, choosing no one, and has to be changed.
    function savedPeople(field, input, people, other, prompt) {
        if (!field || !input || people.length === 0) { return; }
        const select = field.querySelector('select');
        if (prompt) {
            select.append(new Option(prompt, '', true, true));
            select.options[0].disabled = true;
            select.required = true;
        }
        for (const p of people) { select.append(new Option(p.name, p.url)); }
        select.append(new Option(other, ''));
        const choose = () => {
            input.value = select.value;
            input.closest('.field').hidden = select.value !== '' || (Boolean(prompt) && select.selectedIndex === 0);
        };
        select.addEventListener('change', choose);
        field.hidden = false;
        choose();
    }

    // partnerLink offers the member pages, own pages and entries saved in this
    // browser for a synchro partner to say who they are, starting on no one and
    // leaving out the synchro entry itself.
    // The one saved under the partner's name, as entered, is chosen already.
    function partnerLink() {
        const field = document.querySelector('[data-saved-people]');
        const self = (field?.dataset.savedSelf || '').split('\n');
        const input = document.getElementById('partner-link');
        savedPeople(field, input,
            [...loadLinks(linkKeys.members), ...loadLinks(linkKeys.gymnasts), ...loadLinks(linkKeys.entries)].filter((p) => !self.includes(p.name)),
            'Someone else, or no other entry', 'Choose who you are');
        const named = (field?.dataset.partner || '').trim().toLowerCase();
        const select = field?.querySelector('select');
        if (!named || !select || field.hidden) { return; }
        const match = [...select.options].filter((o) => o.value && o.text.split(' · ')[0].trim().toLowerCase() === named);
        if (match.length === 1) {
            select.value = match[0].value;
            select.dispatchEvent(new Event('change'));
        }
    }

    // enterAs offers the own pages, member pages and entries saved in this
    // browser for entering a competition as someone who has a page already.
    // Choosing someone fills in the gymnast's name, the part of the saved
    // name before " · " (unless it's been typed over).
    function enterAs() {
        const field = document.querySelector('[data-saved-as]');
        savedPeople(field, document.getElementById('enter-as'),
            [...loadLinks(linkKeys.gymnasts), ...loadLinks(linkKeys.members), ...loadLinks(linkKeys.entries)], 'Someone new, or another page (paste its link)');
        const name = field?.closest('form')?.querySelector('input[name="gymnast"]');
        const select = field?.querySelector('select');
        if (!name || !select) { return; }
        let filled = '';
        const fill = () => {
            if (name.value !== filled && name.value !== '') { return; }
            const chosen = select.value ? select.selectedOptions[0].text.split(' · ')[0] : '';
            name.value = filled = chosen;
        };
        select.addEventListener('change', fill);
        fill();
    }

    // partnerClubs fill in a synchro partner's club when they're chosen from
    // the club's members (the list on the partner's name), unless typed.
    function partnerClubs() {
        for (const name of document.querySelectorAll('input[name="partnerName"][list]')) {
            const club = name.form.querySelector('input[name="partnerClub"]');
            const members = [...(document.getElementById(name.getAttribute('list'))?.options || [])].map((o) => o.value);
            name.addEventListener('change', () => {
                if (club && club.value === '' && members.includes(name.value)) { club.value = name.dataset.club || ''; }
            });
        }
    }

    // videoChoices select "Video of some skills" when one of its skills is chosen.
    // The tariff's box ticks when a tariff is typed, and unticking it empties
    // the tariff.
    function videoChoices() {
        for (const fieldset of document.querySelectorAll('.comp-video')) {
            const tick = fieldset.querySelector('[data-video-tariff]');
            const tariff = fieldset.querySelector('input[name="videoTariff"]');
            if (tick && tariff) {
                tick.addEventListener('change', () => {
                    if (!tick.checked) { tariff.value = ''; } else { tariff.focus(); }
                });
                tariff.addEventListener('input', () => { tick.checked = tariff.value !== ''; });
            }
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
            // A level opened only because a search found a match in it isn't remembered.
            const now = Array.from(levels).filter((d) => d.open && !('listOpened' in d.dataset)).map((d) => d.dataset.level);
            try { localStorage.setItem(key, JSON.stringify(now)); } catch (e) { /* not remembered */ }
        };
        levels.forEach((d) => d.addEventListener('toggle', save));
    }

    // lazyRows puts rows that wait in a <template data-lazy> (a folded level
    // of the dashboard, views/dashboard.templ) in the page: those inside a
    // <details> when it opens, and every one in root when a search needs
    // them. tables.js hears "lazy-rows" and sets the new rows' columns.
    function lazyRows(root) {
        const waiting = root.querySelectorAll('template[data-lazy]');
        if (waiting.length === 0) { return; }
        waiting.forEach((t) => t.replaceWith(t.content));
        document.dispatchEvent(new CustomEvent('lazy-rows'));
    }
    document.addEventListener('toggle', (e) => {
        if (e.target.tagName === 'DETAILS' && e.target.open) { lazyRows(e.target); }
    }, true);

    // reveal goes to a place in a list marked data-reveal (the timetable's
    // days) that may be folded away: the address's #id after a change, or a
    // link to a day or area. Its rows come into the page, the days around it
    // open, and it scrolls into view.
    function reveal(id) {
        const list = document.querySelector('[data-reveal]');
        if (!list || !id) { return false; }
        const waiting = [...list.querySelectorAll('template[data-lazy]')].find((t) => t.content.getElementById(id));
        if (waiting) {
            const details = waiting.closest('details');
            if (details) { details.open = true; }
            lazyRows(details || list);
        }
        const target = document.getElementById(id);
        if (!target || !list.contains(target)) { return false; }
        for (let d = target.closest('details'); d; d = d.parentElement.closest('details')) {
            // Its rows now, not when the toggle event comes, so the page is
            // as long as it will be before it scrolls.
            if (d.classList.contains('comp-tt-day')) { d.open = true; lazyRows(d); }
        }
        target.scrollIntoView();
        return true;
    }
    document.addEventListener('click', (e) => {
        const a = e.target.closest?.('a[href^="#"]');
        if (a && reveal(decodeURIComponent(a.getAttribute('href').slice(1)))) {
            e.preventDefault();
            try { history.replaceState(history.state, '', a.getAttribute('href')); } catch (err) { /* the address keeps its old place */ }
        }
    });

    // dayPick is the timetable's button for each day (an open day is one
    // shown). On a phone they're tabs: one day at a time. On a wider screen
    // each shows or hides its day, those shown side by side, at least one;
    // the choice is kept in this browser, unless the address goes to a place
    // (after a change), which opens its own day.
    function dayPick() {
        const pick = document.querySelector('.comp-tt-daypick');
        if (!pick) { return; }
        const wide = window.matchMedia('(min-width: 769px)');
        const days = [...document.querySelectorAll('.comp-tt-dayrow > .comp-tt-day')];
        const dayOf = (id) => document.getElementById(id)?.closest('details');
        const key = 'timetableDays:' + location.pathname;
        const row = document.querySelector('.comp-tt-dayrow');
        // tail leaves room after the last day shown when the days don't all
        // fit, so it can scroll to the left edge rather than stop part-way.
        // Measured where the last day ends, not by taking the room away first,
        // which would pull back a row scrolled to the end.
        const tail = () => {
            const shown = days.filter((d) => d.open);
            const last = shown[shown.length - 1];
            let room = 0;
            if (wide.matches && last) {
                const end = last.getBoundingClientRect().right - row.getBoundingClientRect().left - row.clientLeft + row.scrollLeft;
                if (end > row.clientWidth + 1) { room = Math.max(0, row.clientWidth - last.offsetWidth); }
            }
            row.style.setProperty('--tail', room + 'px');
        };
        const sync = () => {
            pick.querySelectorAll('[data-day]').forEach((b) => {
                b.setAttribute('aria-pressed', dayOf(b.dataset.day)?.open ? 'true' : 'false');
            });
            tail();
        };
        window.addEventListener('resize', tail);
        const save = () => {
            const shown = days.filter((d) => d.open).map((d) => d.querySelector('summary').id);
            try { localStorage.setItem(key, JSON.stringify(shown)); } catch (e) { /* this visit only */ }
        };
        if (wide.matches && !location.hash) {
            let kept = null;
            try { kept = JSON.parse(localStorage.getItem(key) || 'null'); } catch (e) { /* none kept */ }
            if (Array.isArray(kept) && days.some((d) => kept.includes(d.querySelector('summary').id))) {
                days.forEach((d) => {
                    d.open = kept.includes(d.querySelector('summary').id);
                    if (d.open) { lazyRows(d); }
                });
            }
        }
        pick.addEventListener('click', (e) => {
            const b = e.target.closest('[data-day]');
            const d = b && dayOf(b.dataset.day);
            if (!d) { return; }
            if (!wide.matches) {
                days.forEach((x) => { x.open = x === d; });
                lazyRows(d);
                sync();
                return;
            }
            if (d.open && days.filter((x) => x.open).length === 1) { return; }
            d.open = !d.open;
            if (d.open) { lazyRows(d); }
            sync();
            save();
        });
        days.forEach((d) => {
            d.addEventListener('toggle', sync);
            // A day's heading is a heading: the buttons show and hide days.
            d.querySelector('summary').addEventListener('click', (e) => e.preventDefault());
        });
        sync();
    }

    // tickBar is the Entries page's bar once entries are ticked: how many, and
    // Remove or Hold, which open the form below with that chosen.
    function tickBar() {
        const bar = document.getElementById('tick-bar');
        const form = document.getElementById('remove-form');
        if (!bar || !form) { return; }
        const boxes = () => [...document.querySelectorAll('input[name="entry"][form="remove-form"]')];
        const update = () => {
            const n = boxes().filter((b) => b.checked).length;
            bar.hidden = n === 0;
            bar.querySelector('[data-tick-count]').textContent = n === 1 ? '1 ticked' : `${n} ticked`;
        };
        document.addEventListener('change', (e) => { if (e.target.closest?.('table')) { update(); } });
        bar.addEventListener('click', (e) => {
            const go = e.target.closest('[data-tick-go]');
            if (go) {
                const choice = form.querySelector(`input[name="action"][value="${go.dataset.tickGo}"]`);
                if (choice) { choice.checked = true; }
                const opens = form.closest('details');
                if (opens) { opens.open = true; }
                form.scrollIntoView({ behavior: 'smooth', block: 'center' });
                form.querySelector('input[name="note"]')?.focus({ preventScroll: true });
            }
            if (e.target.closest('[data-tick-clear]')) {
                document.querySelectorAll('table input[type="checkbox"]').forEach((b) => { if (b.name === 'entry' || /^Tick all/.test(b.getAttribute('aria-label') || '')) { b.checked = false; } });
                update();
            }
        });
        update();
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
        const shownArea = new URLSearchParams(location.search).get('area') || '';
        const key = 'trampolineOpenDay:' + location.pathname + '?day=' + (new URLSearchParams(location.search).get('day') || '') + '&area=' + shownArea;
        // The lists opened by hand, and those "Find a name" opened because
        // they have someone it found (listTools marks them), which close
        // again when the search is cleared and aren't remembered.
        const openKeys = (root) => new Set(Array.from(root.querySelectorAll('details[data-key][open]:not([data-list-opened])'), (d) => d.dataset.key));
        const searchKeys = (root) => new Set(Array.from(root.querySelectorAll('details[data-key][open][data-list-opened]'), (d) => d.dataset.key));
        const reopen = (root, keys) => root.querySelectorAll('details[data-key]').forEach((d) => {
            if ('listOpened' in d.dataset) {
                // Opened by the search, and by hand before: it stays open after it.
                if (keys.has(d.dataset.key)) { delete d.dataset.listOpened; }
                return;
            }
            d.open = keys.has(d.dataset.key);
        });
        // research runs the page's search again, over a fresh copy of the areas.
        const research = () => document.querySelectorAll('[data-list-tools="day-areas"] [data-list-search]').forEach((s) => s.dispatchEvent(new Event('input')));
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

        // show swaps in a fresh copy of the page, keeping the box with the id
        // anchor where it is on the screen; shut is the data-key of a list
        // to leave closed.
        const show = (fresh, anchor, shut) => {
            try {
                // Keep the flight where it is on the screen, whatever changes above it.
                if (!document.getElementById(anchor)) { anchor = 'day-areas'; }
                const before = document.getElementById(anchor).getBoundingClientRect().top;
                const keys = openKeys(document);
                keys.delete(shut);
                const searched = searchKeys(document);
                reopen(fresh, keys);
                fresh.querySelectorAll('details[data-key]').forEach((d) => {
                    if (searched.has(d.dataset.key) && d.dataset.key !== shut) {
                        d.open = true;
                        d.dataset.listOpened = '';
                    }
                });
                swap(fresh);
                // Hide again whoever the search doesn't find.
                research();
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

        // dayOf reads a page as the day page; null if it's something else.
        const dayOf = (text) => {
            const doc = new DOMParser().parseFromString(text, 'text/html');
            return doc.getElementById('day-areas')?.hasAttribute('data-day-page') ? doc : null;
        };

        // send posts the form, and swaps in the page it ends up at.
        const send = async (form, submitter, body, shut) => {
            let fresh;
            try {
                const response = await fetch(form.action, { method: 'POST', body, credentials: 'same-origin' });
                if (!response.ok) { throw new Error('status ' + response.status); }
                fresh = dayOf(await response.text());
                if (!fresh) { throw new Error('not the day page'); }
            } catch (e) {
                plainSubmit(form, submitter);
                return;
            }
            show(fresh, form.closest(submitter?.value === 'finish' ? '.comp-day-area, .comp-day-card' : '.comp-flight, .comp-day-card')?.id || 'day-areas', shut);
        };

        // One at a time, so a slow answer can't undo a newer one.
        let queue = Promise.resolve();

        // Taps (the forms marked data-tap) are kept on the phone first and
        // sent from there (taps.js), so one made without signal isn't lost.
        // Without storage on the phone they're sent as other forms are.
        let storage = null;
        try { storage = window.localStorage; } catch (e) { /* none: sent at once */ }
        const taps = typeof DayTaps === 'undefined' || !storage ? null
            : DayTaps.queue(storage, (url, body) => fetch(url, { method: 'POST', body, credentials: 'same-origin' }), () => Date.now());
        // The flight of the last tap, kept where it is on the screen.
        let tapAnchor = 'day-areas';
        // shownDay is the day the page shows, as its forms post it.
        const shownDay = () => document.querySelector('#day-areas input[name="day"], #day-warnings input[name="day"]')?.value;
        const say = (notices) => {
            const box = document.getElementById('day-notice');
            if (!box || notices.length === 0) { return; }
            box.replaceChildren(...[...new Set(notices)].map((n) => Object.assign(document.createElement('div'), { className: 'notification is-info is-light', textContent: n })));
        };
        const sendTaps = () => {
            if (!taps) { return; }
            queue = queue.then(async () => {
                const flush = () => taps.flush();
                const { answers } = navigator.locks ? await navigator.locks.request('trampolineTaps', flush) : await flush();
                if (answers.length > 0) {
                    // Each answer's notice: done already, refused and why, or
                    // the page the server sent instead of the day's.
                    const notices = [];
                    let fresh = null;
                    for (const a of answers) {
                        const doc = dayOf(a.text);
                        if (doc) {
                            const n = doc.getElementById('day-notice')?.textContent.trim();
                            if (n) { notices.push(n); }
                            const u = new URL(a.url || location.href, location.href);
                            fresh = u.pathname === location.pathname && u.searchParams.get('day') === shownDay() && (u.searchParams.get('area') || '') === shownArea ? doc : null;
                        } else {
                            const page = new DOMParser().parseFromString(a.text, 'text/html');
                            const why = [page.querySelector('h1')?.textContent.trim(), page.querySelector('h1 + p')?.textContent.trim()].filter(Boolean).join(': ');
                            notices.push(`${a.tap.label} wasn't recorded. ${why}`);
                        }
                    }
                    if (!fresh) {
                        // Sent from another day's page: this one, as it is now.
                        try {
                            const response = await fetch(location.href, { credentials: 'same-origin' });
                            fresh = response.ok ? dayOf(await response.text()) : null;
                        } catch (e) { /* no signal again: the page stays as it is */ }
                    }
                    if (fresh) { show(fresh, tapAnchor); }
                    say(notices);
                }
                const left = taps.load();
                DayTaps.markWaiting(left);
                DayTaps.count(left.length);
            });
        };

        document.addEventListener('submit', (event) => {
            const form = event.target;
            if (event.defaultPrevented || !form.closest('#day-areas, #day-warnings')) {
                return;
            }
            event.preventDefault();
            const submitter = event.submitter;
            const tap = taps && DayTaps.tapOf(form, submitter, taps.load());
            if (tap && taps.add(tap)) {
                // Finished moves the flight down to those finished: keep the
                // area where it is instead, so the next flight is in its place.
                const at = submitter?.value === 'finish' ? '.comp-day-area, .comp-day-card' : '.comp-flight, .comp-day-card';
                tapAnchor = form.closest(at)?.id || (form.closest('#day-warnings') ? 'day-warnings' : 'day-areas');
                // Shown as done at once; the count only once it's waiting.
                DayTaps.markWaiting(taps.load());
                if (!navigator.onLine) { DayTaps.count(taps.load().length); }
                sendTaps();
                return;
            }
            // Sent as a plain form (not multipart), as the server expects, with
            // the button that was tapped.
            const body = new URLSearchParams(new FormData(form, submitter));
            if (submitter) { submitter.disabled = true; }
            // A Notify list, or the times put right, close once sent.
            const shut = form.closest('details.comp-notify, details.comp-correct')?.dataset.key;
            queue = queue.then(() => send(form, submitter, body, shut));
        });

        if (taps) {
            // Those kept from before: shown, and sent now, when the signal
            // comes back, and every half minute while any are left.
            DayTaps.markWaiting(taps.load());
            DayTaps.count(taps.load().length);
            sendTaps();
            window.addEventListener('online', sendTaps);
            setInterval(() => { if (taps.load().length > 0) { sendTaps(); } }, 30000);
        }
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

    // fold makes text searchable as the server's NameMatches does: only its
    // letters, lower case, without accents, so "o briain" finds "Ó Briain",
    // "oneill" finds "O'Neill" and "marykate" finds "Mary Kate".
    function fold(text) {
        const plain = { 'ø': 'o', 'ł': 'l', 'đ': 'd', 'ı': 'i' }; // letters without a separate accent
        return text.normalize('NFD').replace(/\p{M}/gu, '').replace(/\P{L}/gu, '').toLowerCase().replace(/[øłđı]/g, (c) => plain[c]);
    }

    // searchWords are a search's words, each folded, as NameMatches reads them.
    function searchWords(text) {
        return text.split(/\s+/).map(fold).filter(Boolean);
    }

    // listTools searches and filters each list of people (views/lists.templ):
    // a row shows when its name has every word searched for and it matches
    // every filter chosen. The search and filters are kept for the tab, so a
    // page that reloads after a change comes back the same. A list the server
    // filters (data-form, the dashboard) sends its filters to the server
    // instead; its search narrows the rows here as it's typed, and the page's
    // address and links marked data-list-carry follow it.
    function listTools() {
        for (const tools of document.querySelectorAll('[data-list-tools]')) {
            if (!document.getElementById(tools.dataset.listTools)) { continue; }
            const form = tools.hasAttribute('data-form');
            // The list and its rows are read afresh each time, as the On the
            // day page swaps in new copies of them (dayPage).
            const readRows = (list) => [...list.querySelectorAll('[data-list-row]')].map((el) => {
                let values = {};
                try { values = JSON.parse(el.dataset.values || '{}'); } catch (e) { /* no values: matches no filter */ }
                return { el, name: fold(el.dataset.name || ''), values };
            });
            const search = tools.querySelector('[data-list-search]');
            const filters = [...tools.querySelectorAll('[data-list-filter]')];
            const count = tools.querySelector('[data-list-count]');
            const clear = tools.querySelector('[data-list-clear]');
            const none = tools.querySelector('[data-list-none]');
            const noun = tools.dataset.noun;
            const stateKey = 'trampolineList:' + location.pathname + ':' + tools.dataset.listTools;

            // matches says whether a row has a filter's chosen value: for an
            // "at least" filter, a value with the same prefix and as high a number.
            // The server has already filtered a form's rows.
            const matches = (row, filter) => {
                if (form) { return true; }
                const have = row.values[filter.dataset.listFilter] || [];
                if (!filter.hasAttribute('data-at-least')) { return have.includes(filter.value); }
                const at = filter.value.lastIndexOf(':');
                const prefix = filter.value.slice(0, at + 1);
                const need = Number(filter.value.slice(at + 1));
                return have.some((v) => v.startsWith(prefix) && Number(v.slice(prefix.length)) >= need);
            };

            // carry puts the search in a URL's q, for the server.
            const carry = (href) => {
                const u = new URL(href, location.href);
                if (search.value.trim() === '') { u.searchParams.delete('q'); } else { u.searchParams.set('q', search.value.trim()); }
                return u.pathname + u.search + u.hash;
            };

            const apply = () => {
                const list = document.getElementById(tools.dataset.listTools);
                if (!list) { return; }
                const words = searchWords(search.value);
                // A search looks through every row, folded ones too.
                if (words.length > 0) { lazyRows(list); }
                const rows = readRows(list);
                let total = form ? Number(tools.dataset.total) : rows.length;
                const chosen = filters.filter((f) => f.value !== '');
                let shown = 0;
                for (const row of rows) {
                    const show = words.every((w) => row.name.includes(w)) && chosen.every((f) => matches(row, f));
                    row.el.hidden = !show;
                    if (show) { shown++; }
                }
                const filtered = words.length > 0 || chosen.length > 0;
                // Rows still folded away (lazyRows) count, unfiltered.
                if (!filtered && list.querySelector('template[data-lazy]')) { shown = total = Number(tools.dataset.total) || shown; }
                // A group of rows (a club's box, a level) hides while filtering
                // finds none of its rows. While searching, a closed group with a
                // match opens, and closes again when the search is cleared.
                for (const group of list.querySelectorAll('[data-list-group]')) {
                    const any = !!group.querySelector('[data-list-row]:not([hidden])');
                    // A level's "12 of 96" follows the search too.
                    const counted = group.querySelector(':scope > summary [data-group-count]');
                    if (counted) {
                        counted.dataset.shown ??= counted.textContent;
                        const n = group.querySelectorAll('[data-list-row]:not([hidden])').length;
                        const of = counted.dataset.groupCount;
                        counted.textContent = words.length === 0 ? counted.dataset.shown : String(n) === of ? of : `${n} of ${of}`;
                    }
                    group.hidden = filtered && !any;
                    if (group.tagName !== 'DETAILS') { continue; }
                    if (words.length > 0 && any && !group.open) {
                        group.dataset.listOpened = '';
                        group.open = true;
                    } else if ((words.length === 0 || !any) && 'listOpened' in group.dataset) {
                        delete group.dataset.listOpened;
                        group.open = false;
                    }
                }
                count.textContent = shown !== total ? `${shown} of ${total} ${noun}` : `${total} ${noun}`;
                const on = tools.querySelector('[data-list-filters-on]');
                if (on) {
                    on.textContent = String(chosen.length);
                    on.hidden = chosen.length === 0;
                }
                clear.hidden = !filtered;
                none.hidden = shown > 0;
                list.hidden = shown === 0;
                if (form) {
                    try { history.replaceState(history.state, '', carry(location.href)); } catch (e) { /* the address keeps the old search */ }
                    for (const a of document.querySelectorAll('a[data-list-carry]')) { a.setAttribute('href', carry(a.getAttribute('href'))); }
                    return;
                }
                try {
                    sessionStorage.setItem(stateKey, JSON.stringify({ q: search.value, f: filters.map((f) => f.value) }));
                } catch (e) { /* not kept: the page starts unfiltered next time */ }
            };

            if (!form) {
                try {
                    const saved = JSON.parse(sessionStorage.getItem(stateKey) || 'null');
                    if (saved) {
                        search.value = saved.q || '';
                        filters.forEach((f, i) => {
                            // A choice the page no longer offers is dropped.
                            if ([...f.options].some((o) => o.value === saved.f?.[i])) { f.value = saved.f[i]; }
                        });
                    }
                } catch (e) { /* nothing kept */ }
                // A link can choose a filter by its key in the address
                // (?coach=…: a coach's members), over what was kept.
                const asked = new URLSearchParams(location.search);
                for (const f of filters) {
                    const v = asked.get(f.dataset.listFilter);
                    if (v !== null && [...f.options].some((o) => o.value === v)) { f.value = v; }
                }
            }
            search.addEventListener('input', apply);
            // Enter sends a form's search to the server, but must not send a
            // form the list sits in (the desk's).
            search.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !form) { e.preventDefault(); } });
            for (const f of filters) { f.addEventListener('change', form ? () => tools.submit() : apply); }
            // A form sends only the filters chosen, keeping the address short.
            tools.addEventListener('formdata', (e) => {
                for (const [k, v] of [...e.formData]) { if (v === '') { e.formData.delete(k); } }
            });
            if (!form) {
                clear.addEventListener('click', () => {
                    search.value = '';
                    for (const f of filters) { f.value = ''; }
                    apply();
                    search.focus();
                });
            }
            tools.hidden = false;
            apply();
        }
    }

    // keepRoutines lists the routines saved in this browser on a member's
    // Routines tab, or a coach's page for a gymnast, each to tick (ticked to
    // start with, unless data-keep-unticked), and posts the ticked ones to keep
    // on the member's page (ADR 0011). Without routines here, or without the
    // script, it stays hidden.
    function keepRoutines() {
        const form = document.querySelector('[data-keep-routines]');
        if (!form || typeof RoutineStore === 'undefined') { return; }
        const routines = RoutineStore.load().routines.filter((r) => r.skills.length > 0);
        if (routines.length === 0) { return; }
        const list = form.querySelector('.comp-keep-list');
        for (const r of routines) {
            const label = Object.assign(document.createElement('label'), { className: 'checkbox is-block mb-1' });
            const box = Object.assign(document.createElement('input'), { type: 'checkbox', value: r.id, checked: !form.hasAttribute('data-keep-unticked') });
            label.append(box, ` ${r.name} (${r.skills.length} ${r.skills.length === 1 ? 'skill' : 'skills'})`);
            list.append(label);
        }
        form.addEventListener('submit', (event) => {
            const ticked = [...list.querySelectorAll('input:checked')].map((box) => routines.find((r) => r.id === box.value));
            if (ticked.length === 0) { event.preventDefault(); return; }
            form.querySelector('input[name="routines"]').value = JSON.stringify(ticked.map(RoutineStore.keepable));
        });
        form.hidden = false;
    }

    // openToAnchor opens the folded lists (details) that hold the element
    // the address's #anchor names, and scrolls it into view: a skill marked
    // in a folded list comes back to it, open, and a routine a coach marked
    // a skill on (#routine-…) is open again for the next.
    function openToAnchor() {
        if (!location.hash.startsWith('#mark-') && !location.hash.startsWith('#routine-')) { return; }
        const target = document.getElementById(location.hash.slice(1));
        if (!target) { return; }
        for (let d = target.closest('details'); d; d = d.parentElement.closest('details')) { d.open = true; }
        target.scrollIntoView({ block: 'center' });
    }

    // sharedLevel reads a shared club level from its link's fragment
    // ("#share=j" and base64url JSON, as the server writes it) and posts it to
    // be shown; on the page that shows it, it offers the coach pages saved in
    // this browser to add it to, members' who coach too included (ADR 0011
    // Decision 6, ADR 0010).
    function sharedLevel() {
        const form = document.querySelector('[data-shared-level]');
        if (form) {
            const hash = location.hash;
            const wait = document.querySelector('[data-shared-level-wait]');
            if (!hash.startsWith('#share=j')) {
                if (wait) { wait.textContent = "This link doesn't carry a level. Ask for it again."; }
                return;
            }
            try {
                const binary = atob(hash.slice('#share=j'.length).replace(/-/g, '+').replace(/_/g, '/'));
                form.querySelector('input[name="level"]').value = new TextDecoder().decode(Uint8Array.from(binary, (c) => c.charCodeAt(0)));
                form.submit();
            } catch (e) {
                if (wait) { wait.textContent = "This link's level couldn't be read. Ask for it again."; }
            }
        }
        const saved = document.querySelector('[data-saved-coaches]');
        if (saved) {
            const coaches = [...loadLinks(linkKeys.coaches), ...loadLinks(linkKeys.members).filter((l) => l.coach)];
            const input = document.getElementById('coach-link');
            if (coaches.length === 0 || !input) { return; }
            const select = saved.querySelector('select');
            for (const c of coaches) { select.append(new Option(c.name, c.url)); }
            select.append(new Option('Another club (paste its link)', ''));
            const pick = () => {
                input.value = select.value;
                input.closest('.field').hidden = select.value !== '';
            };
            select.addEventListener('change', pick);
            pick();
            saved.hidden = false;
        }
    }

    document.addEventListener('DOMContentLoaded', () => {
        keepOffline();
        listTools();
        offlineBanner();
        openLevels();
        menuInView();
        tickBar();
        dayPick();
        dayPage();
        copyButtons();
        saveLinks();
        newCompetition();
        hub();
        clubLink();
        partnerLink();
        enterAs();
        partnerClubs();
        openToAnchor();
        reveal(decodeURIComponent(location.hash.slice(1)));
        forgetDeadLink();
        videoChoices();
        entryForms();
        keepRoutines();
        sharedLevel();
    });
})();
