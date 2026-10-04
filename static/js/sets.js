// sets.js: the requirement sets and levels saved in this browser (ADR 0003),
// shared by the calculator, the tariff sheet and the requirements page. Sets
// are stored as a list of {id, set, described} under
// 'trampolineRequirementSets', where set is the set's JSON and described its
// rules in plain English, for listing. Levels (LevelStore) and the routines
// paired by them (Pairs) follow.
const SetStore = (() => {
    const key = 'trampolineRequirementSets';

    function load() {
        try {
            const sets = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(sets) ? sets.filter((s) => s && s.id && s.set) : [];
        } catch (e) {
            return [];
        }
    }

    function save(sets) {
        try { localStorage.setItem(key, JSON.stringify(sets)); } catch (e) { console.error('Could not save requirement sets:', e); }
    }

    const newId = () => 'set-' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

    // payload is what the server needs to check a routine: a built-in reference
    // ("builtin:<id>") as it is, or a saved set's JSON ('' for none or unknown).
    function payload(ref) {
        if (!ref) { return ''; }
        if (ref.startsWith('builtin:')) { return ref; }
        const saved = load().find((s) => s.id === ref);
        return saved ? JSON.stringify(saved.set) : '';
    }

    return { load, save, newId, payload };
})();

// LevelStore: the levels saved in this browser, stored as a list of {id, level}
// under 'trampolineLevels'. A level pairs requirements for a competition
// level's two exercises (requirements.Level); its options refer to built-in
// requirements or to sets in SetStore by id.
const LevelStore = (() => {
    const key = 'trampolineLevels';

    function load() {
        try {
            const levels = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(levels) ? levels.filter((l) => l && l.id && l.level) : [];
        } catch (e) {
            return [];
        }
    }

    function save(levels) {
        try { localStorage.setItem(key, JSON.stringify(levels)); } catch (e) { console.error('Could not save levels:', e); }
    }

    const newId = () => 'level-' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

    // payload is what the server needs: a built-in reference as it is, or a
    // saved level's JSON ('' for none or unknown).
    function payload(ref) {
        if (!ref) { return ''; }
        if (ref.startsWith('builtin-level:')) { return ref; }
        const saved = load().find((l) => l.id === ref);
        return saved ? JSON.stringify(saved.level) : '';
    }

    return { key, load, save, newId, payload };
})();

// Pairs: the routines doing a level's two exercises. A routine doing one has
// level (the level's reference), exercise (1 or 2), requirements (the option
// it's checked against) and, once there is one, partner: the id of the routine
// doing the other exercise, which points back.
const Pairs = {
    // partnerOf is the routine doing the other exercise of routine's level, if
    // the two still point at each other at the same level.
    partnerOf(routine, routines) {
        if (!routine?.level || !routine.partner) { return undefined; }
        const partner = routines.find((r) => r.id === routine.partner);
        return partner && partner.partner === routine.id && partner.level === routine.level ? partner : undefined;
    },

    // values are what /routine and /tariff-sheet need to check routine: its
    // skills, requirements and checks, and with a level, the level, the
    // exercise and the routine doing the other one.
    values(routine, routines) {
        const values = {
            routineData: JSON.stringify(routine?.skills || []),
            requirementSet: SetStore.payload(routine?.requirements),
            checks: JSON.stringify(routine?.checks || {}),
        };
        if (!routine?.level) { return values; }
        values.level = LevelStore.payload(routine.level);
        values.exercise = String(routine.exercise === 2 ? 2 : 1);
        const partner = Pairs.partnerOf(routine, routines);
        if (partner) {
            values.pairData = JSON.stringify(partner.skills);
            values.pairSet = SetStore.payload(partner.requirements);
            values.pairChecks = JSON.stringify(partner.checks || {});
            values.pairName = partner.name;
        }
        return values;
    },
};
