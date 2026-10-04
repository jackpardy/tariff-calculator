// view.js: the view screen (views.ViewPage). Shows a routine saved in this
// browser (?routine=<id>, else the current one), the routine doing its level's
// other exercise and the level's other set routine options, rendered by the
// server to fill the screen. It
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
            const partner = Pairs.partnerOf(r, state.routines);
            const option = new Option(partner ? `${r.name} + ${partner.name}` : r.name, r.id, false, r.id === routine.id);
            return option;
        }));
        document.title = routine.name;
        htmx.ajax('POST', '/view', {
            source: '#display', target: '#display', swap: 'innerHTML',
            values: { ...Pairs.values(routine, state.routines), routineName: routine.name, ...optionValues(routine, state.routines) },
        });
    }

    // optionValues say which of its level's options a routine and its partner
    // do, and carry the custom requirements a custom level uses, so the server
    // can show the set routine options no routine is doing.
    function optionValues(routine, routines) {
        if (!routine.level) { return {}; }
        const values = { optionRef: routine.requirements || '', pairOptionRef: Pairs.partnerOf(routine, routines)?.requirements || '' };
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
