// competitions.js: the competition and club pages (views/competitions.templ,
// views/clubs.templ). Copies links; keeps organisers', comp secs', members'
// and gymnasts' links in this browser and lists them; offers the levels saved
// here when creating a competition, and the clubs saved here when entering
// one; fills each voluntary on an entry form from the routines saved here;
// and remembers which levels of the organiser's dashboard are open.
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

    document.addEventListener('DOMContentLoaded', () => {
        openLevels();
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
