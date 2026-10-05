// competitions.js: the competition pages (views/competitions.templ). Copies
// links, keeps an organiser's admin links and a gymnast's entry links in this
// browser, offers the levels saved here when creating a competition, and fills
// each voluntary on the entry form from the routines saved here.
(function () {
    // Links saved in this browser: {name, url} lists under these keys.
    const linkKeys = { competitions: 'trampolineCompetitions', entries: 'trampolineEntryLinks' };

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
    }

    // newCompetition lists this browser's competitions and offers its own levels,
    // each posted as {level, sets}: the level and copies of the requirements it
    // names, which the server keeps with the competition.
    function newCompetition() {
        const saved = document.getElementById('saved-competitions');
        const competitions = loadLinks(linkKeys.competitions);
        if (saved && competitions.length > 0) {
            const list = saved.querySelector('ul');
            for (const c of competitions) {
                const a = document.createElement('a');
                a.href = c.url;
                a.textContent = c.name;
                const item = document.createElement('li');
                item.append(a);
                list.append(item);
            }
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

    document.addEventListener('DOMContentLoaded', () => {
        copyButtons();
        saveLinks();
        newCompetition();
        entryForms();
    });
})();
