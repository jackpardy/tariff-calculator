// sets.js: the requirement sets and levels saved in this browser (ADR 0003),
// shared by the calculator, the tariff sheet and the requirements page. Sets
// are stored as a list of {id, set, described} under
// 'trampolineRequirementSets', where set is the set's JSON and described its
// rules in plain English, for listing. Levels (LevelStore), their exercises
// and tabs (Exercises), and the levels a coach is working on (LevelEntries)
// follow.
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

// Exercises: a level's exercises and their tabs, one per option (e.g. BUCS
// L7: Set 1, Set 2, Voluntary), shared by the builder's Levels mode and the
// view screen.
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
    // tabs are a level's tabs, in order: {exercise, ref, set, label, key}. Set
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
                out.push({ exercise: n, ref, set, label, key: `${n}:${ref}` });
            }
        }
        const voluntaryTabs = out.filter((t) => t.label === 'Voluntary');
        if (voluntaryTabs.length > 1) { voluntaryTabs.forEach((t) => { t.label = t.exercise === 1 ? '1st voluntary' : '2nd voluntary'; }); }
        return out;
    },

    // values are what /routine, /tariff-sheet and /view need to check a routine
    // against its requirements.
    values(routine) {
        return {
            routineData: JSON.stringify(routine?.skills || []),
            requirementSet: SetStore.payload(routine?.requirements),
            checks: JSON.stringify(routine?.checks || {}),
        };
    },
};

// LevelEntries: the levels a coach is working on, saved in this browser as
// {current, entries} under 'trampolineLevelEntries'. An entry is {id, name,
// level, exercises: [{option, routine}, {option, routine}], open, beside}:
// option is the requirements chosen for each exercise (the set routine
// performed, or the voluntary's), routine the id of the routine doing that
// exercise's voluntary, and open and beside the keys of the tabs shown
// ("<exercise>:<ref>"). Set routines aren't stored: they're shown as
// prescribed. Several entries can be for the same level, e.g. one per gymnast.
const LevelEntries = {
    key: 'trampolineLevelEntries',
    load() {
        let state;
        try { state = JSON.parse(localStorage.getItem(this.key) || 'null'); } catch (e) { state = null; }
        const entries = Array.isArray(state?.entries) ? state.entries.filter((e) => e && e.id && e.level) : [];
        for (const e of entries) {
            e.exercises = Array.isArray(e.exercises) ? e.exercises : [];
            while (e.exercises.length < 2) { e.exercises.push({}); }
        }
        return { current: entries.some((e) => e.id === state?.current) ? state.current : entries[0]?.id || null, entries };
    },
    save(state) {
        try { localStorage.setItem(this.key, JSON.stringify(state)); } catch (e) { console.error('Could not save levels:', e); }
    },
    newId() { return 'entry-' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6); },

    // chosen is the option an entry's exercise n is on: the one chosen, or the
    // exercise's first.
    chosen(entry, n) {
        const options = Exercises.options(Exercises.findLevel(entry.level), n);
        const option = entry.exercises?.[n - 1]?.option;
        return options.includes(option) ? option : options[0];
    },
    // routineFor is the routine doing exercise n's voluntary, if there is one.
    routineFor(entry, n, routines) {
        const id = entry.exercises?.[n - 1]?.routine;
        return id ? routines.find((r) => r.id === id) : undefined;
    },

    // values are what the server needs to check exercise n of an entry with
    // the other one: the level, the exercise, and the other exercise's chosen
    // option and skills (empty for a set routine, which the server builds).
    values(entry, n, routines) {
        const level = Exercises.findLevel(entry.level);
        const values = { level: LevelStore.payload(entry.level), exercise: String(n) };
        if (Exercises.count(level) < 2) { return values; }
        const m = n === 2 ? 1 : 2;
        const ref = this.chosen(entry, m);
        const routine = Exercises.isSetRoutine(ref) ? undefined : this.routineFor(entry, m, routines);
        values.pairData = routine ? JSON.stringify(routine.skills) : '';
        values.pairSet = SetStore.payload(ref);
        values.pairChecks = JSON.stringify(routine?.checks || {});
        values.pairName = Exercises.tabs(level).find((t) => t.exercise === m && t.ref === ref)?.label || '';
        return values;
    },

    // migrate turns routines that held a level's exercises as tabs (an earlier
    // version) into level entries: each voluntary with skills becomes (or stays)
    // a routine of its own, linked to the entry. It needs the page's level data.
    migrate(routineState) {
        const old = routineState.routines.filter((r) => r.level);
        if (old.length === 0) { return false; }
        const state = this.load();
        for (const r of old) {
            const level = Exercises.findLevel(r.level);
            const entry = { id: this.newId(), name: level ? r.name : `${r.name} (level no longer saved)`, level: r.level, exercises: [{}, {}] };
            const openN = r.exercise === 2 ? 2 : 1;
            let reused = false;
            for (const tab of Exercises.tabs(level)) {
                const open = tab.exercise === openN && tab.ref === r.requirements;
                const slot = open ? { skills: r.skills, checks: r.checks } : r.exercises?.[tab.exercise - 1]?.slots?.[tab.ref];
                if (r.exercises?.[tab.exercise - 1]?.option === tab.ref || open) { entry.exercises[tab.exercise - 1].option = tab.ref; }
                if (tab.set || !slot?.skills?.length) { continue; }
                let routine = r;
                if (reused) {
                    routine = { id: RoutineStore.newId(), name: `${r.name} · ${tab.label}`.slice(0, 60), skills: slot.skills };
                    routineState.routines.splice(routineState.routines.indexOf(r) + 1, 0, routine);
                } else {
                    r.skills = slot.skills;
                    reused = true;
                }
                routine.requirements = tab.ref;
                if (slot.checks) { routine.checks = slot.checks; } else { delete routine.checks; }
                entry.exercises[tab.exercise - 1].routine = routine.id;
            }
            // A routine that only held set routines stays, checked against the one on screen.
            delete r.level;
            delete r.exercise;
            delete r.exercises;
            entry.open = Exercises.tabs(level).find((t) => t.exercise === openN && t.ref === entry.exercises[openN - 1].option)?.key;
            state.entries.push(entry);
        }
        state.current = state.current || state.entries[0]?.id || null;
        this.save(state);
        RoutineStore.save(routineState);
        return true;
    },
};
