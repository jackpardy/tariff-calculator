// routines.js: the routines saved in this browser, shared by the calculator, the
// tariff sheet and the compare page. Each routine is {id, name, skills}; one is
// current. They are stored as {current, routines} under 'trampolineRoutines'.
// The single routine older versions saved (under 'trampolineRoutine') becomes
// "Routine 1".
const RoutineStore = (() => {
    const key = 'trampolineRoutines';
    const legacyKey = 'trampolineRoutine';

    const newId = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 7);

    function read(k) {
        try { return JSON.parse(localStorage.getItem(k) || 'null'); } catch (e) { return null; }
    }

    // nextName is the first "Routine N" not already used.
    function nextName(routines) {
        const used = new Set(routines.map((r) => r.name));
        for (let n = 1; ; n++) {
            if (!used.has(`Routine ${n}`)) { return `Routine ${n}`; }
        }
    }

    // load returns the saved routines, always with at least one, and a valid current id.
    function load() {
        let state = read(key);
        if (!state || !Array.isArray(state.routines)) {
            const legacy = read(legacyKey);
            state = { current: null, routines: [] };
            if (Array.isArray(legacy) && legacy.length > 0) {
                state.routines.push({ id: newId(), name: 'Routine 1', skills: legacy });
            }
        }
        state.routines = state.routines.filter((r) => r && Array.isArray(r.skills));
        if (state.routines.length === 0) {
            state.routines.push({ id: newId(), name: 'Routine 1', skills: [] });
        }
        if (!state.routines.some((r) => r.id === state.current)) {
            state.current = state.routines[0].id;
        }
        return state;
    }

    function save(state) {
        try {
            localStorage.setItem(key, JSON.stringify(state));
            localStorage.removeItem(legacyKey);
        } catch (e) {
            console.error('Could not save routines:', e);
        }
    }

    const current = (state) => state.routines.find((r) => r.id === state.current);

    return { load, save, current, newId, nextName };
})();
