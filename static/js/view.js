// view.js: the view screen (views.ViewPage). Shows a routine saved in this
// browser (?routine=<id>, else the current one): for a level routine, both its
// exercises and the level's other set routine options, rendered by the server
// to fill the screen. It
// re-renders when the routines change in another tab (the builder). The Show
// menu's boxes (data-hides) toggle classes on #view, remembered in this
// browser, and the full screen button uses the Fullscreen API where there is one.
(function () {
    const storageKey = (box) => 'viewShow:' + box.dataset.hides;

    function remembered(key) {
        try { return localStorage.getItem(key); } catch (e) { return null; }
    }
    function remember(key, value) {
        try { localStorage.setItem(key, value); } catch (e) { /* storage unavailable: still works for this visit */ }
    }

    // chosen is the routine to show: the one asked for, or the current one.
    function chosen(state) {
        const id = new URLSearchParams(location.search).get('routine');
        return state.routines.find((r) => r.id === id) || RoutineStore.current(state);
    }

    function render() {
        const state = RoutineStore.load();
        const routine = chosen(state);
        const select = document.getElementById('view-routine');
        select.replaceChildren(...state.routines.map((r) => {
            const level = Exercises.findLevel(r.level);
            return new Option(level ? `${r.name} · ${level.name}` : r.name, r.id, false, r.id === routine.id);
        }));
        document.title = routine.name;
        htmx.ajax('POST', '/view', {
            source: '#display', target: '#display', swap: 'innerHTML',
            values: { ...Exercises.values(routine), routineName: Exercises.tabFor(routine)?.label || routine.name, ...optionValues(routine) },
        });
    }

    // optionValues say which of its level's options the routine's two
    // exercises are on, what each tab is called, and carry the custom
    // requirements a custom level uses, so the server can show the set routine
    // options not chosen as well.
    function optionValues(routine) {
        if (!routine.level) { return {}; }
        const tabNames = {};
        for (const tab of Exercises.tabs(Exercises.findLevel(routine.level))) { tabNames[tab.ref] = tab.label; }
        const values = { optionRef: routine.requirements || '', pairOptionRef: Exercises.other(routine)?.ref || '', tabNames: JSON.stringify(tabNames) };
        const level = LevelStore.load().find((l) => l.id === routine.level)?.level;
        if (level) {
            const sets = {};
            for (const exercise of [level.first, level.second]) {
                for (const ref of exercise?.options || []) {
                    const payload = SetStore.payload(ref);
                    if (!ref.startsWith('builtin:') && payload) { sets[ref] = JSON.parse(payload); }
                }
            }
            values.optionSets = JSON.stringify(sets);
        }
        return values;
    }

    document.addEventListener('DOMContentLoaded', () => {
        const view = document.getElementById('view');

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
            const params = new URLSearchParams(location.search);
            params.set('routine', event.target.value);
            history.replaceState(null, '', `${location.pathname}?${params}`);
            render();
        });
        window.addEventListener('storage', (event) => {
            if (event.key === 'trampolineRoutines' || event.key === 'trampolineRequirementSets' || event.key === LevelStore.key) { render(); }
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

        render();
    });
})();
