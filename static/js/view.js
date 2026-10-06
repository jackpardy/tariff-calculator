// view.js: the view screen (views.ViewPage). Shows a routine saved in this
// browser (?routine=<id>, else the current one), with another beside it
// (&beside=<id>) as in the builder, or a level being worked on (?entry=<id>):
// its set routines and voluntaries, rendered by the server to fill the screen. It re-renders when the routines or levels change in another
// tab (the builder). The Show menu lists the routines on screen to show or
// hide, and its other boxes (data-hides) toggle classes on #view, all
// remembered in this browser. The full screen button uses the Fullscreen API
// where there is one.
(function () {
    const storageKey = (box) => 'viewShow:' + box.dataset.hides;

    function remembered(key) {
        try { return localStorage.getItem(key); } catch (e) { return null; }
    }
    function remember(key, value) {
        try { localStorage.setItem(key, value); } catch (e) { /* storage unavailable: still works for this visit */ }
    }

    // chosen is what to show: {entry} for a level asked for, else {routine}:
    // the one asked for, or the current one, and {beside} if one goes beside it.
    function chosen(state, levels) {
        const params = new URLSearchParams(location.search);
        const entry = levels.entries.find((e) => e.id === params.get('entry'));
        if (entry) { return { entry, id: entry.id, name: entry.name }; }
        const routine = state.routines.find((r) => r.id === params.get('routine')) || RoutineStore.current(state);
        const beside = state.routines.find((r) => r.id === params.get('beside') && r.id !== routine.id);
        if (beside) { return { routine, beside, id: `${routine.id}+${beside.id}`, name: `${routine.name} + ${beside.name}` }; }
        return { routine, id: routine.id, name: routine.name };
    }

    function render() {
        const state = RoutineStore.load(), levels = LevelEntries.load();
        const shown = chosen(state, levels);
        const select = document.getElementById('view-routine');
        select.replaceChildren(
            ...(shown.beside ? [new Option(shown.name, `pair:${shown.routine.id}:${shown.beside.id}`, false, true)] : []),
            ...levels.entries.map((e) => new Option(`${e.name} (level)`, `entry:${e.id}`, false, e.id === shown.id)),
            ...state.routines.map((r) => new Option(r.name, `routine:${r.id}`, false, r.id === shown.id)),
        );
        document.title = shown.name;
        let values;
        if (shown.entry) {
            values = entryValues(shown.entry, state.routines);
        } else {
            values = { ...Exercises.values(shown.routine), routineName: shown.routine.name };
            if (shown.beside) {
                const beside = Exercises.values(shown.beside);
                Object.assign(values, { besideData: beside.routineData, besideSet: beside.requirementSet, besideChecks: beside.checks, besideName: shown.beside.name });
            }
        }
        htmx.ajax('POST', '/view', { source: '#display', target: '#display', swap: 'innerHTML', values });
    }

    // entryValues show a level: its first exercise's choice (a set routine as
    // prescribed, or the voluntary's routine) with the second's, and the other
    // set routine options. They say which option each exercise is on, what
    // each tab is called, and carry the custom requirements a custom level uses.
    function entryValues(entry, routines) {
        const level = Exercises.findLevel(entry.level);
        const tabs = Exercises.tabs(level);
        const ref = LevelEntries.chosen(entry, 1);
        const tab = tabs.find((t) => t.exercise === 1 && t.ref === ref);
        const routine = tab?.set ? undefined : LevelEntries.routineFor(entry, 1, routines);
        const own = tab?.set
            ? { prescribed: '1', requirementSet: SetStore.payload(ref), routineData: '', checks: '{}' }
            : { ...Exercises.values(routine), requirementSet: SetStore.payload(ref) };
        const tabNames = {};
        for (const t of tabs) { tabNames[t.ref] = tabNames[t.ref] || t.label; }
        const values = {
            ...own, ...LevelEntries.values(entry, 1, routines),
            routineName: tab?.label || 'First exercise',
            optionRef: ref || '', pairOptionRef: Exercises.count(level) > 1 ? LevelEntries.chosen(entry, 2) || '' : '',
            tabNames: JSON.stringify(tabNames),
        };
        const custom = LevelStore.load().find((l) => l.id === entry.level)?.level;
        if (custom) {
            const sets = {};
            for (const exercise of [custom.first, custom.second]) {
                for (const option of exercise?.options || []) {
                    const payload = SetStore.payload(option);
                    if (!option.startsWith('builtin:') && payload) { sets[option] = JSON.parse(payload); }
                }
            }
            values.optionSets = JSON.stringify(sets);
        }
        return values;
    }

    // showColumns lists the routines on screen in the Show menu, ticked unless
    // hidden. What's hidden is remembered per routine or level
    // ('viewHidden:<id>'), by column key (exercise and option).
    function showColumns() {
        const shown = chosen(RoutineStore.load(), LevelEntries.load());
        const key = 'viewHidden:' + shown.id;
        let hidden = [];
        try { hidden = JSON.parse(remembered(key) || '[]'); } catch (e) { hidden = []; }
        const columns = [...document.querySelectorAll('.display-column[data-column]')];
        const list = document.getElementById('view-columns');
        const apply = () => {
            for (const c of columns) { c.hidden = hidden.includes(c.dataset.column); }
            // Difficulty options only mean something when a routine shown scores
            // difficulty (set routines usually don't).
            const scored = columns.some((c) => !c.hidden && c.dataset.scored === 'true');
            for (const key of ['diff', 'total']) {
                const option = document.querySelector(`.view-menu-panel [data-option="${key}"]`);
                if (option) { option.hidden = !scored; }
            }
        };
        list.replaceChildren(...columns.map((c) => {
            const box = Object.assign(document.createElement('input'), { type: 'checkbox', checked: !hidden.includes(c.dataset.column) });
            box.addEventListener('change', () => {
                hidden = hidden.filter((k) => k !== c.dataset.column);
                if (!box.checked) { hidden.push(c.dataset.column); }
                remember(key, JSON.stringify(hidden));
                apply();
            });
            const label = document.createElement('label');
            label.append(box, ` ${c.dataset.name}`);
            return label;
        }));
        list.hidden = columns.length < 2;
        apply();
    }

    document.addEventListener('DOMContentLoaded', () => {
        const view = document.getElementById('view');
        document.body.addEventListener('htmx:afterSwap', (event) => {
            if (event.detail.target.id === 'display') { showColumns(); }
        });

        // The Show menu.
        const boxes = [...document.querySelectorAll('input[data-hides]')];
        const apply = () => { for (const box of boxes) { view.classList.toggle(box.dataset.hides, !box.checked); } };
        for (const box of boxes) {
            const saved = remembered(storageKey(box));
            if (saved !== null) { box.checked = saved === 'true'; }
            box.addEventListener('change', () => { remember(storageKey(box), String(box.checked)); apply(); });
        }
        apply();

        // Choosing a routine.
        document.getElementById('view-routine').addEventListener('change', (event) => {
            const [kind, id, beside] = event.target.value.split(':');
            const params = new URLSearchParams();
            if (kind === 'pair') {
                params.set('routine', id);
                params.set('beside', beside);
            } else {
                params.set(kind, id);
            }
            history.replaceState(null, '', `${location.pathname}?${params}`);
            render();
        });
        window.addEventListener('storage', (event) => {
            if (['trampolineRoutines', 'trampolineRequirementSets', LevelStore.key, LevelEntries.key].includes(event.key)) { render(); }
        });

        // Share what's on screen: the level, or the routine (and the one beside it).
        document.getElementById('view-share').addEventListener('click', () => {
            const shown = chosen(RoutineStore.load(), LevelEntries.load());
            if (shown.entry) {
                Share.open('entries', [shown.entry.id]);
            } else {
                Share.open('routines', [shown.routine.id, shown.beside?.id].filter(Boolean));
            }
        });

        // Full screen, where the browser allows it (not on iPhone).
        const full = document.getElementById('view-fullscreen');
        if (document.fullscreenEnabled) {
            full.hidden = false;
            full.addEventListener('click', () => {
                if (document.fullscreenElement) { document.exitFullscreen(); } else { document.documentElement.requestFullscreen().catch(() => {}); }
            });
            document.addEventListener('fullscreenchange', () => {
                full.textContent = document.fullscreenElement ? 'Exit full screen' : 'Full screen';
            });
        }

        LevelEntries.migrate(RoutineStore.load()); // routines that held a level's tabs become level entries
        render();
    });
})();
