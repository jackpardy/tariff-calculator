// sets.js: the requirement sets and levels saved in this browser (ADR 0003),
// shared by the calculator, the tariff sheet and the requirements page. Sets
// are stored as a list of {id, set, described} under
// 'trampolineRequirementSets', where set is the set's JSON and described its
// rules in plain English, for listing. Levels (LevelStore) and a level
// routine's exercises (Exercises) follow.
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

// Exercises: a routine checked against a level holds both of its exercises, one
// tab per option (e.g. BUCS L7: Set 1, Set 2, Voluntary). Its level is the
// level's reference, exercise (1 or 2) the open tab's exercise, and
// requirements the open tab's option. skills and checks are the open tab's;
// the others' are kept in exercises: [{option, slots: {<ref>: {skills,
// checks}}}, ...], where option is the option chosen for that exercise (the
// set routine performed, say). A set routine's tab starts as the set routine.
const Exercises = {
    // data is the page's built-in levels and requirements (views.LevelData in
    // #level-data): {levels, names, set_routines}, read once.
    data() {
        if (!this._data) {
            try { this._data = JSON.parse(document.getElementById('level-data').textContent); } catch (e) { this._data = {}; }
        }
        return this._data;
    },
    findLevel(ref) {
        if (!ref) { return undefined; }
        if (ref.startsWith('builtin-level:')) { return this.data().levels?.[ref]; }
        return LevelStore.load().find((l) => l.id === ref)?.level;
    },
    isSetRoutine(ref) {
        if (ref?.startsWith('builtin:')) { return (this.data().set_routines || []).includes(ref); }
        const saved = SetStore.load().find((s) => s.id === ref);
        return (saved?.set.rules || []).some((r) => r.type === 'sequence');
    },
    requirementName(ref) {
        if (ref?.startsWith('builtin:')) { return this.data().names?.[ref] || ref; }
        return SetStore.load().find((s) => s.id === ref)?.set.name || 'Requirements no longer saved';
    },
    options(level, n) { return ((n === 2 && level?.second) || level?.first)?.options || []; },
    // count is how many exercises a level has to fill in: two, unless both are
    // the same set routine (BG Club), when there's one.
    count(level) {
        if (!level) { return 0; }
        return level.second || !this.options(level, 1).every((ref) => this.isSetRoutine(ref)) ? 2 : 1;
    },
    // tabs are a level's tabs, in order: {exercise, ref, set, label}. Set
    // routines are "Set 1", "Set 2" (or "Set routine" alone); a voluntary is
    // "Voluntary", or "1st voluntary" and "2nd voluntary" when both are, or
    // named after its requirements when an exercise offers several.
    tabs(level) {
        const out = [];
        for (let n = 1; n <= this.count(level); n++) {
            const options = this.options(level, n);
            const sets = options.filter((ref) => this.isSetRoutine(ref));
            const voluntaries = options.filter((ref) => !this.isSetRoutine(ref));
            for (const ref of options) {
                const set = sets.includes(ref);
                let label;
                if (set) {
                    label = sets.length > 1 ? `Set ${sets.indexOf(ref) + 1}` : 'Set routine';
                } else if (voluntaries.length > 1) {
                    label = this.requirementName(ref);
                } else {
                    label = 'Voluntary';
                }
                out.push({ exercise: n, ref, set, label });
            }
        }
        const voluntaryTabs = out.filter((t) => t.label === 'Voluntary');
        if (voluntaryTabs.length > 1) { voluntaryTabs.forEach((t) => { t.label = t.exercise === 1 ? '1st voluntary' : '2nd voluntary'; }); }
        return out;
    },
    tabFor(routine, n = routine?.exercise === 2 ? 2 : 1) {
        const level = this.findLevel(routine?.level);
        const ref = n === (routine?.exercise === 2 ? 2 : 1) ? routine?.requirements : routine?.exercises?.[n - 1]?.option;
        return this.tabs(level).find((t) => t.exercise === n && t.ref === ref);
    },
    // slot is a tab's skills and checks: the routine's own for the open tab,
    // otherwise what's kept in exercises (undefined if never opened).
    slot(routine, n, ref) {
        if ((routine.exercise === 2 ? 2 : 1) === n && routine.requirements === ref) { return { skills: routine.skills, checks: routine.checks }; }
        return routine.exercises?.[n - 1]?.slots?.[ref];
    },
    // other is the other exercise of a level routine: {exercise, ref, skills,
    // checks}, or undefined when the level has one exercise.
    other(routine) {
        const level = this.findLevel(routine?.level);
        if (!level || this.count(level) < 2) { return undefined; }
        const n = routine.exercise === 2 ? 1 : 2;
        const ref = routine.exercises?.[n - 1]?.option || this.options(level, n)[0];
        return { exercise: n, ref, ...(this.slot(routine, n, ref) || {}) };
    },

    // values are what /routine, /tariff-sheet and /view need to check a routine:
    // its skills, requirements and checks, and with a level, the level, the
    // open tab's exercise and the other exercise (whose skills, if a set
    // routine never opened, the server builds from its requirements).
    values(routine) {
        const values = {
            routineData: JSON.stringify(routine?.skills || []),
            requirementSet: SetStore.payload(routine?.requirements),
            checks: JSON.stringify(routine?.checks || {}),
        };
        if (!routine?.level) { return values; }
        values.level = LevelStore.payload(routine.level);
        values.exercise = String(routine.exercise === 2 ? 2 : 1);
        const other = this.other(routine);
        if (other) {
            values.pairData = other.skills ? JSON.stringify(other.skills) : '';
            values.pairSet = SetStore.payload(other.ref);
            values.pairChecks = JSON.stringify(other.checks || {});
            const tab = this.tabFor(routine, other.exercise);
            values.pairName = tab?.label || '';
        }
        return values;
    },
};
