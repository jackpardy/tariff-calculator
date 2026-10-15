// routines.js: the routines saved in this browser, shared by the calculator, the
// tariff sheet, the view screen and the compare page. Each routine is {id, name,
// skills}, with the requirements and checks it's checked against; one is
// current. Levels are kept apart (LevelEntries in sets.js) and link routines.
// They are stored as {current, routines, beside} under 'trampolineRoutines',
// beside being the routine the builder shows side by side, if any. The single
// routine older versions saved (under 'trampolineRoutine') becomes "Routine 1".
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
        // Routines paired by level in an earlier version lose the pairing; the
        // page turns level routines into level entries (LevelEntries.migrate).
        for (const r of state.routines) {
            if (r.level && !Array.isArray(r.exercises)) {
                r.exercises = [{}, {}];
                r.exercises[r.exercise === 2 ? 1 : 0].option = r.requirements;
            }
            delete r.partner;
        }
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

    // keepable is a routine as a member's page keeps it (ADR 0011): its id
    // here, so keeping it again changes it, its name, its skills without the
    // names and tariffs the server works out again, what it's checked
    // against (a built-in reference or its own requirements) and its checks.
    function keepable(r) {
        const skills = r.skills.map(({ name, tariff, ...skill }) => skill);
        const requirements = typeof SetStore === 'undefined' ? '' : SetStore.payload(r.requirements);
        return { id: r.id, name: r.name, skills, requirements, checks: r.checks || null };
    }

    // keep posts routines to a member's page (its full link), which opens it.
    function keep(memberLink, routines) {
        const form = Object.assign(document.createElement('form'), { method: 'post', action: `${memberLink}/routines` });
        form.append(Object.assign(document.createElement('input'), { type: 'hidden', name: 'routines', value: JSON.stringify(routines.map(keepable)) }));
        document.body.append(form);
        form.submit();
    }

    // savedLinks are the club pages of one kind saved in this browser ({name,
    // url}), as the competition pages save them (static/js/competitions.js):
    // under key, each a link to path.
    function savedLinks(key, path) {
        try {
            const links = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(links) ? links.filter((l) => l && typeof l.url === 'string' && l.url.includes(path)) : [];
        } catch (e) {
            return [];
        }
    }

    // memberLinks are the member pages saved in this browser.
    const memberLinks = () => savedLinks('trampolineMemberLinks', '/clubs/member/');

    // coachLinks are the pages saved in this browser that open a coach's
    // pages: coaches' own, and members' who coach too (ADR 0010).
    const coachLinks = () => [...savedLinks('trampolineCoachLinks', '/clubs/coach/'), ...memberLinks().filter((l) => l.coach)];

    return { load, save, current, newId, nextName, keepable, keep, memberLinks, coachLinks };
})();
