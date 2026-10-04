// app.js: the calculator page's Alpine component (x-data="tariffCalculatorStore()").
// Loaded with defer before Alpine, so the function exists when Alpine starts.
// The page's state: the routines saved in this browser (RoutineStore, from
// routines.js), the current one's skills, and UI state local to the page.
// Everything shown is rendered by the server; this component asks for it
// whenever the routine or the form changes.
//
// A second routine can be shown beside the current one (compareId), and both
// can be edited. Methods that act on a routine's skills take the column's side:
// 'a' for the current routine (the default), 'b' for the one beside it.
function tariffCalculatorStore() {
    return {
        routines: [],   // all saved routines: {id, name, skills}
        currentId: null,
        routine: [],    // the current routine's skills (the same array as in routines)
        customSets: [], // requirement sets saved in this browser (SetStore)
        customLevels: [], // levels saved in this browser (LevelStore)
        expanded: [], // per-card expanded state, parallel to routine
        compareId: null, // the routine shown beside the current one, if any
        routineB: [],    // its skills (the same array as in routines)
        expandedB: [],   // per-card expanded state, parallel to routineB
        addTo: '',       // where Add puts a skill (an addTargets value); '' for the routine on screen
        mobileTab: 'a',  // the column shown on a phone while comparing
        setRoutineOffer: null, // a set routine to load, waiting for "new routine" or "replace": {side, name, skills, ref, previous}
        editingIndex: null,
        editingSide: 'a', // the column of the skill being edited
        busy: false, // an add or update is in flight
        pickerTab: 'jumps', // the skill picker's open category
        picked: null,       // the picker entry last chosen (catalog.Entry ID)
        pickedShape: null,  // the shape it was chosen in
        query: '',          // the picker's search text; results replace the tabs while it's set
        toast: { show: false, message: '', type: 'info' },

        init() {
            const state = RoutineStore.load();
            this.routines = state.routines;
            this.currentId = state.current;
            this.routine = this.currentRoutine().skills;
            this.persist(); // settles a routine carried over from an older version
            this.customSets = SetStore.load();
            this.customLevels = LevelStore.load();
            // Sets and levels edited in another tab (the requirements page) are picked up here.
            window.addEventListener('storage', (event) => {
                if (event.key === 'trampolineRequirementSets' || event.key === LevelStore.key) {
                    this.customSets = SetStore.load();
                    this.customLevels = LevelStore.load();
                    this.renderRoutines();
                }
            });
            // One flag per card from the start, so moves can splice it in step with the routine.
            this.expanded = this.routine.map(() => false);

            this.$watch('routine', (routine) => {
                this.currentRoutine().skills = routine; // the routine may have been replaced (e.g. cleared)
                this.renderRoutines();
                this.persist();
            });
            this.$watch('routineB', (routine) => {
                if (!this.compareId) { return; }
                this.compareRoutine().skills = routine;
                this.renderRoutines();
                this.persist();
            });

            document.body.addEventListener('htmx:afterSwap', (event) => {
                if (event.detail.target.id === 'routine-view') { this.makeSortable('a'); }
                if (event.detail.target.id === 'routine-view-b') { this.makeSortable('b'); }
            });
            document.body.addEventListener('htmx:afterRequest', (event) => {
                if (event.detail.failed) { this.requestFailed(event.detail.requestConfig?.path, event.detail.xhr); }
            });

            this.renderRoutines();
            this.loadForm();
        },

        // --- Server-rendered views ---
        // renderRoutine shows a column's routine; while comparing, the other column's
        // routine goes with it so the view can mark where they differ.
        renderRoutine(side = 'a') {
            const view = side === 'b' ? '#routine-view-b' : '#routine-view';
            const values = { side, ...Exercises.values(this.routineFor(side)) };
            if (this.compareId) {
                const other = side === 'b' ? 'a' : 'b';
                values.compareData = JSON.stringify(this.skillsOf(other));
                values.compareName = this.routineFor(other).name;
            }
            // Each view is its own request source, so its requests don't queue behind
            // the other's; a newer render replaces one still in flight (hx-sync).
            htmx.ajax('POST', '/routine', { source: view, target: view, swap: 'innerHTML', values })
                .catch(error => console.error('Routine render request error:', error));
        },
        // renderRoutines shows both columns: a change to either changes what differs.
        renderRoutines() {
            this.renderRoutine('a');
            if (this.compareId) { this.renderRoutine('b'); }
        },
        // loadForm shows a fresh "Add a skill" panel, or one loaded with the skill at editIndex in a column.
        loadForm(editIndex = null, side = 'a') {
            const values = {};
            if (editIndex !== null) {
                values.skill = JSON.stringify(this.skillsOf(side)[editIndex]);
                values.editIndex = editIndex;
            }
            return htmx.ajax('POST', '/skill-form', { source: '#skill-form-wrapper', target: '#skill-form-wrapper', swap: 'innerHTML', values });
        },
        requestFailed(path, xhr) {
            const reason = xhr.status === 400 ? xhr.responseText.trim() : 'the server could not be reached';
            if (path === '/routine') {
                this.showToast(`Routine not updated: ${reason}`, 'error');
                // If nothing has rendered yet (e.g. a saved routine that no longer validates), say so in place.
                for (const view of document.querySelectorAll('#routine-view, #routine-view-b')) {
                    if (view.querySelector('.routine-skills') || view.closest('[x-show]')?.style.display === 'none') { continue; }
                    const p = document.createElement('p');
                    p.className = 'has-text-danger';
                    p.textContent = `Couldn't show the saved routine (${reason}). Use Clear skills to start again.`;
                    view.replaceChildren(p);
                }
            } else {
                this.showToast(`Request failed: ${reason}`, 'error');
            }
        },

        // boxTariff is a picker box's tariff: the base (tuck) one, or once the box is
        // picked, the chosen shape's, which its shape buttons are compared with.
        boxTariff(tariffsJSON, id, base) {
            const tariffs = JSON.parse(tariffsJSON);
            const picked = this.picked === id ? tariffs[this.pickedShape] : undefined;
            return (picked ?? base).toFixed(1);
        },
        // shapeModifier is how a picker box's shape differs in tariff from tuck, or,
        // once that box is picked, from the shape chosen: "+0.1", "−0.1" or "".
        shapeModifier(tariffsJSON, shape, id) {
            const tariffs = JSON.parse(tariffsJSON);
            const from = this.picked === id && tariffs[this.pickedShape] !== undefined ? tariffs[this.pickedShape] : tariffs.Tuck;
            const diff = Math.round((tariffs[shape] - from) * 10) / 10;
            if (diff > 0) { return `+${diff.toFixed(1)}`; }
            if (diff < 0) { return `−${(-diff).toFixed(1)}`; }
            return '';
        },

        // step nudges a number input (the − and + buttons) and lets the form re-render.
        step(id, delta) {
            const input = document.getElementById(id);
            if (!input) { return; }
            const min = parseInt(input.min), max = parseInt(input.max);
            let value = (parseInt(input.value) || 0) + delta;
            if (!isNaN(min)) { value = Math.max(min, value); }
            if (!isNaN(max)) { value = Math.min(max, value); }
            if (String(value) === input.value) { return; }
            input.value = value;
            input.dispatchEvent(new Event('change', { bubbles: true }));
        },

        // --- Saved routines ---
        currentRoutine() { return this.routines.find((r) => r.id === this.currentId); },
        compareRoutine() { return this.routines.find((r) => r.id === this.compareId); },
        routineFor(side) { return side === 'b' ? this.compareRoutine() : this.currentRoutine(); },
        // isSetRoutine reports whether a requirement set is a set (prescribed) routine.
        isSetRoutine(set) { return (set?.rules || []).some((r) => r.type === 'sequence'); },
        skillsOf(side) { return side === 'b' ? this.routineB : this.routine; },
        expandedOf(side) { return side === 'b' ? this.expandedB : this.expanded; },
        persist() { RoutineStore.save({ current: this.currentId, routines: this.routines }); },
        // setRequirements chooses the requirement set a column's routine is checked
        // against. A set (compulsory) routine is loaded into the builder.
        setRequirements(ref, side = 'a') {
            const routine = this.routineFor(side);
            const previous = this.snapshot(routine);
            routine.requirements = ref || undefined;
            delete routine.checks; // the new set's checks apply
            this.persist();
            this.renderRoutine(side);
            if (ref) { this.loadSetRoutine(side, false, previous); }
        },
        // setCheck turns one of a column's checks ('difficulty' or 'repeats') on or
        // off for its routine, overriding its requirement set's choice.
        setCheck(side, check, on) {
            const routine = this.routineFor(side);
            routine.checks = { ...routine.checks, [check]: on };
            this.persist();
            this.renderRoutines();
        },
        // snapshot is what a routine is checked against, so it can go back to it.
        snapshot(routine) { return { ref: routine.requirements, checks: routine.checks }; },
        // loadSetRoutine loads a column's requirement set's set routine: straight in
        // if the routine is empty, otherwise offering a new routine or replacing
        // this one's skills (setRoutineOffer). Sets without a set routine are left
        // alone, quietly unless the user asked (fromButton). previous is what the
        // routine was checked against before (a snapshot), which it gets back if
        // the set routine goes into a new routine instead.
        async loadSetRoutine(side = 'a', fromButton = false, previous = undefined) {
            const routine = this.routineFor(side);
            if (!routine?.requirements) { return; }
            if (routine.level) {
                const skills = await this.fetchSetRoutine(routine.requirements);
                if (skills) { this.setSkills(side, skills); }
                return;
            }
            const body = new URLSearchParams({ requirementSet: SetStore.payload(routine.requirements), routineData: JSON.stringify(this.skillsOf(side)) });
            let loaded;
            try {
                const response = await fetch('/set-routine', { method: 'POST', body });
                if (!response.ok) {
                    if (fromButton) { this.showToast(`Couldn't load the set routine: ${(await response.text()).trim()}`, 'error'); }
                    return;
                }
                loaded = await response.json();
            } catch (error) {
                if (fromButton) { this.showToast("Couldn't load the set routine.", 'error'); }
                return;
            }
            if (loaded.matches) { return; }
            const offer = { side, name: loaded.name, skills: loaded.skills, ref: routine.requirements, previous: fromButton ? this.snapshot(routine) : previous };
            if (this.skillsOf(side).length === 0) { this.replaceWithSetRoutine(offer); return; }
            this.setRoutineOffer = offer;
        },
        // replaceWithSetRoutine puts an offered set routine in place of a column's skills.
        replaceWithSetRoutine(offer = this.setRoutineOffer) {
            this.setRoutineOffer = null;
            const { side } = offer;
            if (this.editingSide === side && this.editingIndex !== null) { this.cancelEdit(); }
            const expanded = this.expandedOf(side), skills = this.skillsOf(side);
            expanded.splice(0, expanded.length, ...offer.skills.map(() => false));
            skills.splice(0, skills.length, ...JSON.parse(JSON.stringify(offer.skills))); // re-renders and saves via the watcher
            this.showToast(`Loaded the set routine into ${this.routineFor(side).name}.`, 'info');
        },
        // newRoutineFromSetRoutine puts an offered set routine in a new routine, named
        // after the set and checked against it, shown in the same column. The routine
        // the set was chosen for goes back to what it was checked against.
        newRoutineFromSetRoutine(offer = this.setRoutineOffer) {
            this.setRoutineOffer = null;
            const from = this.routineFor(offer.side), previous = offer.previous || {};
            from.requirements = previous.ref || undefined;
            if (previous.checks) { from.checks = previous.checks; } else { delete from.checks; }
            const routine = { id: RoutineStore.newId(), name: (offer.name || RoutineStore.nextName(this.routines)).slice(0, 60), skills: JSON.parse(JSON.stringify(offer.skills)), requirements: offer.ref };
            this.routines.splice(this.routines.indexOf(from) + 1, 0, routine);
            this.persist();
            if (offer.side === 'b') {
                this.compareWith(routine.id);
            } else {
                this.switchRoutine(routine.id);
            }
            this.showToast(`Started ${routine.name}.`, 'info');
        },
        // cancelSetRoutine leaves the routine's skills alone; it stays checked
        // against the set, so the panel still offers to load it.
        cancelSetRoutine() { this.setRoutineOffer = null; },

        // --- Levels: one routine, a tab per exercise option (Exercises, in sets.js) ---
        // chooseCheck is the "Check against" choice: a level ("level:<ref>") or
        // requirements on their own, which take the routine out of any level
        // (select is put back if the coach keeps the level).
        chooseCheck(value, side = 'a', select = undefined) {
            if (value.startsWith('level:')) { this.setLevel(value.slice('level:'.length), side); return; }
            const routine = this.routineFor(side);
            if (!this.leaveLevel(routine)) {
                if (select) { select.value = `level:${routine.level}`; }
                return;
            }
            this.setRequirements(value, side);
        },
        findLevel(ref) { return Exercises.findLevel(ref); },
        levelOf(side) { return this.findLevel(this.routineFor(side)?.level); },
        exerciseOf(side) { return this.routineFor(side)?.exercise === 2 ? 2 : 1; },
        tabsFor(side) {
            this.customSets; this.customLevels; // re-evaluate when these change
            return Exercises.tabs(this.levelOf(side));
        },
        isOpenTab(side, tab) { return this.exerciseOf(side) === tab.exercise && this.routineFor(side)?.requirements === tab.ref; },
        // isChosenTab marks the option chosen for its exercise (the set routine
        // performed), when the exercise offers more than one.
        isChosenTab(side, tab) {
            const routine = this.routineFor(side);
            if (Exercises.options(this.levelOf(side), tab.exercise).length < 2) { return false; }
            const chosen = tab.exercise === this.exerciseOf(side) ? routine.requirements : routine.exercises?.[tab.exercise - 1]?.option;
            return chosen === tab.ref;
        },
        openTabOf(side) { return this.tabsFor(side).find((t) => this.isOpenTab(side, t)); },
        exerciseState(routine, n) {
            routine.exercises = Array.isArray(routine.exercises) ? routine.exercises : [];
            while (routine.exercises.length < 2) { routine.exercises.push({}); }
            return routine.exercises[n - 1];
        },
        // setSkills puts skills in a column's routine (and on screen).
        setSkills(side, skills) {
            if (this.editingSide === side && this.editingIndex !== null) { this.cancelEdit(); }
            this.routineFor(side).skills = skills;
            if (side === 'b') {
                this.expandedB = skills.map(() => false);
                this.routineB = skills;
            } else {
                this.expanded = skills.map(() => false);
                this.routine = skills;
            }
            this.persist();
            this.renderRoutines();
        },
        // fetchSetRoutine is the skills of a set routine's requirements, or undefined.
        async fetchSetRoutine(ref) {
            try {
                const response = await fetch('/set-routine', { method: 'POST', body: new URLSearchParams({ requirementSet: SetStore.payload(ref) }) });
                return response.ok ? (await response.json()).skills : undefined;
            } catch (e) {
                return undefined;
            }
        },
        // stashTab keeps the open tab's skills and checks while another is open.
        stashTab(routine) {
            const exercise = this.exerciseState(routine, routine.exercise === 2 ? 2 : 1);
            exercise.option = routine.requirements;
            exercise.slots = exercise.slots || {};
            exercise.slots[routine.requirements] = { skills: routine.skills, ...(routine.checks ? { checks: routine.checks } : {}) };
        },
        // openTab opens one of a level routine's tabs: its skills as left, or a set
        // routine as prescribed. Opening an option makes it the exercise's choice.
        async openTab(side, tab) {
            const routine = this.routineFor(side);
            if (!tab || this.isOpenTab(side, tab)) { return; }
            this.stashTab(routine);
            const exercise = this.exerciseState(routine, tab.exercise);
            const saved = exercise.slots?.[tab.ref];
            if (saved) { delete exercise.slots[tab.ref]; }
            exercise.option = tab.ref;
            routine.exercise = tab.exercise;
            routine.requirements = tab.ref;
            if (saved?.checks) { routine.checks = saved.checks; } else { delete routine.checks; }
            let skills = saved?.skills;
            if (!skills && tab.set) { skills = await this.fetchSetRoutine(tab.ref); }
            this.setSkills(side, skills || []);
        },
        // setLevel checks a column's routine against a level. Skills already in it
        // become the level's voluntary; otherwise it opens on the first tab. A
        // routine with skills isn't made into a level of set routines only: a new
        // routine is started for that.
        async setLevel(ref, side = 'a') {
            const routine = this.routineFor(side);
            const tabs = Exercises.tabs(this.findLevel(ref));
            if (routine.level === ref || tabs.length === 0) { return; }
            const voluntary = tabs.find((t) => !t.set && t.exercise === 2) || tabs.find((t) => !t.set);
            if (routine.skills.length > 0 && !voluntary) {
                const fresh = { id: RoutineStore.newId(), name: this.findLevel(ref).name.slice(0, 60), skills: [] };
                this.routines.splice(this.routines.indexOf(routine) + 1, 0, fresh);
                if (side === 'b') { this.compareWith(fresh.id); } else { this.switchRoutine(fresh.id); }
                this.showToast(`Started ${fresh.name}; ${routine.name} is unchanged.`, 'info');
                await this.setLevel(ref, side);
                return;
            }
            const start = routine.skills.length > 0 ? voluntary : tabs[0];
            routine.level = ref;
            routine.exercises = [{}, {}];
            for (const n of [1, 2]) { routine.exercises[n - 1].option = Exercises.options(this.findLevel(ref), n)[0]; }
            routine.exercises[start.exercise - 1].option = start.ref;
            routine.exercise = start.exercise;
            routine.requirements = start.ref;
            delete routine.checks;
            if (routine.skills.length === 0 && start.set) {
                this.setSkills(side, (await this.fetchSetRoutine(start.ref)) || []);
            } else {
                this.persist();
                this.renderRoutines();
            }
        },
        // leaveLevel takes a routine out of its level, keeping the open tab's
        // skills. It asks first if other tabs have skills of the coach's own, and
        // reports whether it went ahead.
        leaveLevel(routine) {
            if (!routine.level) { return true; }
            const lost = Exercises.tabs(this.findLevel(routine.level)).filter((t) => !t.set && !(t.exercise === (routine.exercise === 2 ? 2 : 1) && t.ref === routine.requirements))
                .filter((t) => Exercises.slot(routine, t.exercise, t.ref)?.skills?.length > 0);
            if (lost.length > 0 && !confirm(`Stop checking against the level? ${lost.map((t) => t.label).join(' and ')} will be removed; the skills shown now stay.`)) { return false; }
            delete routine.level;
            delete routine.exercise;
            delete routine.exercises;
            return true;
        },
        // copyTarget is the voluntary a set routine tab can be copied into: the
        // other exercise's (its chosen one), or any voluntary.
        copyTarget(side) {
            const open = this.openTabOf(side);
            if (!open?.set) { return undefined; }
            const voluntaries = this.tabsFor(side).filter((t) => !t.set);
            const pool = voluntaries.filter((t) => t.exercise !== open.exercise);
            const from = pool.length > 0 ? pool : voluntaries;
            const routine = this.routineFor(side);
            return from.find((t) => routine.exercises?.[t.exercise - 1]?.option === t.ref) || from[0];
        },
        // copyToVoluntary starts the voluntary from the set routine on screen, and opens it.
        async copyToVoluntary(side) {
            const target = this.copyTarget(side);
            if (!target) { return; }
            const routine = this.routineFor(side);
            const existing = Exercises.slot(routine, target.exercise, target.ref)?.skills || [];
            if (existing.length > 0 && !confirm(`Replace the ${existing.length} skills in ${target.label} with this set routine?`)) { return; }
            const exercise = this.exerciseState(routine, target.exercise);
            exercise.slots = exercise.slots || {};
            exercise.slots[target.ref] = { skills: JSON.parse(JSON.stringify(routine.skills)) };
            await this.openTab(side, target);
            this.showToast(`Copied into ${target.label}; change it from here.`, 'info');
        },
        // switchRoutine makes the routine with id current, leaving any edit. Choosing
        // the routine shown beside it swaps the columns.
        switchRoutine(id) {
            if (!this.routines.some((r) => r.id === id)) { return; }
            if (id === this.compareId) { this.swapColumns(); return; }
            if (this.editingIndex !== null) { this.cancelEdit(); }
            this.addTo = ''; // Add follows the routine on screen
            this.currentId = id;
            this.expanded = this.currentRoutine().skills.map(() => false);
            this.routine = this.currentRoutine().skills; // re-renders and saves via the watcher
        },
        newRoutine() {
            const routine = { id: RoutineStore.newId(), name: RoutineStore.nextName(this.routines), skills: [] };
            this.routines.push(routine);
            this.switchRoutine(routine.id);
            this.showToast(`Started ${routine.name}.`, 'info');
        },
        duplicateRoutine() {
            const source = this.currentRoutine();
            const copy = { ...JSON.parse(JSON.stringify(source)), id: RoutineStore.newId(), name: `${source.name} (copy)`.slice(0, 60) };
            this.routines.splice(this.routines.indexOf(source) + 1, 0, copy);
            this.switchRoutine(copy.id);
            this.showToast(`Duplicated as ${copy.name}.`, 'info');
        },
        renameRoutine() {
            const routine = this.currentRoutine();
            const name = prompt('Name this routine', routine.name);
            if (name === null || name.trim() === '') { return; }
            routine.name = name.trim().slice(0, 60);
            this.persist();
        },
        // --- Side by side ---
        // compareWith shows the routine with id beside the current one ('' stops).
        compareWith(id) {
            if (!id || id === this.currentId || !this.routines.some((r) => r.id === id)) { this.stopComparing(); return; }
            if (this.editingSide === 'b' && this.editingIndex !== null) { this.cancelEdit(); }
            this.compareId = id;
            this.expandedB = this.compareRoutine().skills.map(() => false);
            this.routineB = this.compareRoutine().skills; // renders both columns via the watcher
        },
        stopComparing() {
            if (this.editingSide === 'b' && this.editingIndex !== null) { this.cancelEdit(); }
            this.compareId = null;
            this.routineB = [];
            this.expandedB = [];
            this.mobileTab = 'a';
            this.renderRoutine('a');
        },
        // swapColumns makes the routine beside the current one current, and vice versa.
        swapColumns() {
            if (!this.compareId) { return; }
            [this.currentId, this.compareId] = [this.compareId, this.currentId];
            [this.expanded, this.expandedB] = [this.expandedB, this.expanded];
            const flip = (side) => (side === 'a' ? 'b' : 'a');
            this.editingSide = flip(this.editingSide);
            this.mobileTab = flip(this.mobileTab); // keep showing the same routine
            this.routine = this.currentRoutine().skills;
            this.routineB = this.compareRoutine().skills;
        },

        deleteRoutine() {
            const routine = this.currentRoutine(), name = routine.name;
            if (!confirm(`Delete ${name}? This can't be undone.`)) { return; }
            this.stopComparing();
            if (this.routines.length === 1) {
                // Always keep one routine: emptying the last one is the same as clearing it.
                routine.name = 'Routine 1';
                this.routine = [];
                this.expanded = [];
            } else {
                const index = this.routines.indexOf(routine);
                this.routines.splice(index, 1);
                this.switchRoutine(this.routines[Math.max(0, index - 1)].id);
            }
            this.showToast(`Deleted ${name}.`, 'info');
        },

        // --- Card expand/collapse ---
        toggleExpanded(index, side = 'a') { const e = this.expandedOf(side); e[index] = !e[index]; },
        expandAll() { this.expanded = this.routine.map(() => true); },
        collapseAll() { this.expanded = this.routine.map(() => false); },

        // --- Adding and editing skills ---
        // calculateFormSkill posts the form and returns the skill as the routine stores it.
        async calculateFormSkill() {
            const form = document.getElementById('main-form');
            const response = await fetch('/calculate-skill', { method: 'POST', body: new URLSearchParams(new FormData(form)) });
            if (!response.ok) { throw new Error((await response.text()).trim() || `HTTP ${response.status}`); }
            return response.json();
        },
        // addTargets are the routines Add can put a skill in, for the "Add to"
        // list: every saved routine, and for a level routine, each voluntary tab
        // (and its open tab). A value is a routine id, or "<id>|<exercise>|<ref>"
        // for a level routine's tab.
        addTargets() {
            const out = [];
            for (const r of this.routines) {
                const tabs = Exercises.tabs(this.findLevel(r.level));
                if (tabs.length === 0) {
                    out.push({ value: r.id, label: `${r.name} (${r.skills.length})` });
                    continue;
                }
                for (const t of tabs) {
                    const open = (r.exercise === 2 ? 2 : 1) === t.exercise && r.requirements === t.ref;
                    if (t.set && !open) { continue; }
                    const count = (Exercises.slot(r, t.exercise, t.ref)?.skills || []).length;
                    out.push({ value: `${r.id}|${t.exercise}|${t.ref}`, label: `${r.name} · ${t.label} (${count})` });
                }
            }
            return out;
        },
        // addToValue is where Add will put a skill: the one chosen, or the open
        // tab of the routine on screen.
        addToValue() {
            if (this.addTo && this.addTargets().some((t) => t.value === this.addTo)) { return this.addTo; }
            const r = this.currentRoutine();
            return r?.level ? `${r.id}|${r.exercise === 2 ? 2 : 1}|${r.requirements}` : r?.id;
        },
        // addTarget resolves addToValue: the routine, its label, and the column
        // it's shown in ('a' or 'b'), if it's on screen as it is.
        addTarget() {
            const value = this.addToValue();
            const [id, exercise, ref] = (value || '').split('|');
            const routine = this.routines.find((r) => r.id === id);
            if (!routine) { return undefined; }
            const n = Number(exercise) || undefined;
            const open = !ref || ((routine.exercise === 2 ? 2 : 1) === n && routine.requirements === ref);
            const side = !open ? undefined : id === this.currentId ? 'a' : id === this.compareId ? 'b' : undefined;
            const label = this.addTargets().find((t) => t.value === value)?.label.replace(/ \(\d+\)$/, '') || routine.name;
            return { routine, exercise: n, ref, open, side, label };
        },
        // addFromForm adds the skill shown on the card to the end of the routine
        // chosen under "Add to" (the one on screen unless another is chosen).
        async addFromForm() {
            if (this.busy) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                const target = this.addTarget();
                if (!target) { throw new Error('choose a routine to add to'); }
                let skills;
                if (target.side) {
                    skills = this.skillsOf(target.side);
                    this.expandedOf(target.side).push(false);
                    skills.push(skill);
                    this.mobileTab = target.side; // show where it went
                } else if (target.open) {
                    skills = target.routine.skills;
                    skills.push(skill);
                    this.persist();
                } else {
                    const exercise = this.exerciseState(target.routine, target.exercise);
                    exercise.slots = exercise.slots || {};
                    exercise.slots[target.ref] = exercise.slots[target.ref] || { skills: [] };
                    skills = exercise.slots[target.ref].skills;
                    skills.push(skill);
                    this.persist();
                    this.renderRoutines(); // the level panel counts the other exercise
                }
                const where = target.side === 'a' && !this.compareId ? '' : ` to ${target.label}`;
                this.showToast(`Added ${skill.custom_name || skill.name} (${skill.tariff.toFixed(1)})${where}.`, 'info');
                if (skills.length > 10) { this.showToast('Note: an exercise has 10 skills.', 'warning'); }
                this.clearLabel(); // a label belongs to one skill
                this.query = '';   // back to the picker for the next skill
            } catch (error) {
                this.showToast(`Can't add skill: ${error.message}`, 'error');
            } finally {
                this.busy = false;
            }
        },
        async updateFromForm() {
            const index = this.editingIndex, skills = this.skillsOf(this.editingSide);
            if (this.busy || index === null || index >= skills.length) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                skills.splice(index, 1, skill);
                this.cancelEdit();
            } catch (error) {
                this.showToast(`Can't update skill: ${error.message}`, 'error');
            } finally {
                this.busy = false;
            }
        },
        clearLabel() {
            const input = document.getElementById('custom-name');
            if (!input) { return; }
            input.value = '';
            input.closest('details')?.removeAttribute('open');
        },
        editSkill(index, side = 'a') {
            this.editingIndex = index;
            this.editingSide = side;
            this.loadForm(index, side)
                .then(() => document.getElementById('skill-form-wrapper')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
                .catch(() => { this.showToast('Failed to load edit form.', 'error'); this.editingIndex = null; });
        },
        cancelEdit() {
            this.editingIndex = null;
            this.editingSide = 'a';
            this.loadForm().catch(() => this.showToast('Failed to load the form.', 'error'));
        },

        // --- Choosing which elements score (when only some do) ---
        // toggleScores ticks or unticks an element to score. The first choice starts
        // from the elements scoring now (the highest), as the list records them.
        toggleScores(index, side, on) {
            const skills = this.skillsOf(side);
            if (!skills[index]) { return; }
            if (!skills.some((s) => s.scores)) {
                const list = document.getElementById(side === 'b' ? 'routine-skills-b' : 'routine-skills');
                JSON.parse(list?.dataset.scoring || '[]').forEach((k) => { if (skills[k]) { skills[k].scores = true; } });
            }
            if (on) { skills[index].scores = true; } else { delete skills[index].scores; }
        },
        // clearScores goes back to the highest elements scoring.
        clearScores(side) { this.skillsOf(side).forEach((s) => { delete s.scores; }); },

        // --- Changing the routine ---
        removeSkill(index, side = 'a') {
            const skills = this.skillsOf(side);
            if (index < 0 || index >= skills.length) { return; }
            this.expandedOf(side).splice(index, 1);
            skills.splice(index, 1);
            if (this.editingSide !== side) { return; }
            if (this.editingIndex === index) { this.cancelEdit(); }
            else if (this.editingIndex > index) { this.editingIndex--; }
        },
        moveSkillUp(index, side = 'a') { if (index > 0) { this.moveSkill(index, index - 1, side); } },
        moveSkillDown(index, side = 'a') { if (index < this.skillsOf(side).length - 1) { this.moveSkill(index, index + 1, side); } },
        // moveSkill moves the skill at from to index to, keeping its expanded and editing state with it.
        moveSkill(from, to, side = 'a') {
            const expanded = this.expandedOf(side), skills = this.skillsOf(side);
            expanded.splice(to, 0, expanded.splice(from, 1)[0] ?? false);
            skills.splice(to, 0, skills.splice(from, 1)[0]);
            if (this.editingSide !== side || this.editingIndex === null) { return; }
            if (this.editingIndex === from) { this.editingIndex = to; }
            else if (from < this.editingIndex && to >= this.editingIndex) { this.editingIndex--; }
            else if (from > this.editingIndex && to <= this.editingIndex) { this.editingIndex++; }
        },
        clearRoutine() {
            if (this.routine.length === 0 || !confirm(`Remove every skill from ${this.currentRoutine().name}?`)) { return; }
            this.routine = [];
            this.expanded = [];
            this.showToast('Routine cleared.', 'info');
            if (this.editingSide === 'a') { this.cancelEdit(); }
        },

        // --- Drag to reorder ---
        // makeSortable lets a column's cards be dragged into a new order (SortableJS).
        // The view is re-rendered after every change, so this runs for each new list.
        makeSortable(side = 'a') {
            const list = document.getElementById(side === 'b' ? 'routine-skills-b' : 'routine-skills');
            if (!list || !window.Sortable) { return; }
            Sortable.create(list, {
                draggable: '.routine-skill-container',
                filter: 'button', preventOnFilter: false, // the card's buttons stay clickable
                delay: 200, delayOnTouchOnly: true,       // on touch, press and hold so the page still scrolls
                animation: 150,
                onEnd: (event) => {
                    const from = event.oldDraggableIndex, to = event.newDraggableIndex;
                    if (from === undefined || from === to) { return; }
                    this.moveSkill(from, to, side);
                },
            });
        },

        showToast(message, type = 'info') {
            this.toast = { show: true, message, type };
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(() => this.toast.show = false, 3000);
        },
    }
}
