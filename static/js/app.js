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
//
// In Levels mode the columns are a level's tabs (LevelEntries, in sets.js): a
// set routine, shown as prescribed, or a voluntary, which is an ordinary routine
// linked to the level and shown as the current (or compared) routine.
function tariffCalculatorStore() {
    return {
        routines: [],   // all saved routines: {id, name, skills}
        currentId: null,
        routine: [],    // the current routine's skills (the same array as in routines)
        customSets: [], // requirement sets saved in this browser (SetStore)
        customLevels: [], // levels saved in this browser (LevelStore)
        mode: 'routines', // 'routines', or 'levels' to work on a level's exercises
        levelState: { current: null, entries: [] }, // the levels being worked on (LevelEntries)
        expanded: [], // per-card expanded state, parallel to routine
        compareId: null, // the routine shown beside the current one, if any
        routineB: [],    // its skills (the same array as in routines)
        expandedB: [],   // per-card expanded state, parallel to routineB
        addTo: '',       // the routine Add puts a skill in (its id); '' for the one on screen
        mobileTab: 'a',  // the column shown on a phone while comparing
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
            LevelEntries.migrate(state); // routines that held a level's tabs become level entries
            // A set routine is a starting point now, not requirements to check against.
            for (const r of state.routines) {
                if (r.requirements && Exercises.isSetRoutine(r.requirements)) {
                    delete r.requirements;
                    delete r.checks;
                }
            }
            this.routines = state.routines;
            this.currentId = state.current;
            this.routine = this.currentRoutine().skills;
            this.persist(); // settles a routine carried over from an older version
            this.customSets = SetStore.load();
            this.customLevels = LevelStore.load();
            this.levelState = LevelEntries.load();
            try { this.mode = localStorage.getItem('builderMode') === 'levels' ? 'levels' : 'routines'; } catch (e) { /* routines */ }
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

            if (this.mode === 'levels') { this.syncColumns(); } else { this.renderRoutines(); }
            this.loadForm();
        },

        // --- Server-rendered views ---
        // renderRoutine shows a column's routine; while comparing, the other column's
        // routine goes with it so the view can mark where they differ.
        renderRoutine(side = 'a') {
            const view = side === 'b' ? '#routine-view-b' : '#routine-view';
            const tab = this.columnTab(side);
            let values;
            if (tab) {
                // A level's tab: its set routine as prescribed, or the routine doing its voluntary.
                const routine = tab.set ? undefined : this.columnRoutine(side);
                if (!tab.set && !routine) {
                    const p = document.querySelector(view);
                    if (p) { p.innerHTML = '<p class="has-text-grey level-empty">No routine for this voluntary yet. Choose one above: a new routine, one you\'ve built, or start from a set.</p>'; }
                    return;
                }
                const entryValues = LevelEntries.values(this.entry(), tab.exercise, this.routines);
                values = tab.set
                    ? { side, prescribed: '1', requirementSet: SetStore.payload(tab.ref), ...entryValues }
                    : { side, ...Exercises.values(routine), ...entryValues };
            } else {
                values = { side, ...Exercises.values(this.routineFor(side)) };
            }
            if (this.mode === 'routines' && this.compareId) {
                const other = side === 'b' ? 'a' : 'b';
                values.compareData = JSON.stringify(this.skillsOf(other));
                values.compareName = this.routineFor(other).name;
            }
            // Each view is its own request source, so its requests don't queue behind
            // the other's; a newer render replaces one still in flight (hx-sync).
            htmx.ajax('POST', '/routine', { source: view, target: view, swap: 'innerHTML', values })
                .catch((error) => { if (error) { console.error('Routine render request error:', error); } }); // undefined: replaced by a newer render
        },
        // renderRoutines shows both columns: a change to either changes what differs.
        // Calls made together (e.g. switching tabs, then the watchers that follows)
        // render once.
        renderRoutines() {
            if (this._renderQueued) { return; }
            this._renderQueued = true;
            setTimeout(() => {
                this._renderQueued = false;
                this.renderRoutine('a');
                if (this.twoColumns()) { this.renderRoutine('b'); }
            });
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
        // setRequirements chooses the requirements a column's routine is checked against.
        setRequirements(ref, side = 'a') {
            const routine = this.routineFor(side);
            routine.requirements = ref || undefined;
            delete routine.checks; // the new requirements' checks apply
            this.persist();
            this.renderRoutine(side);
        },
        // setCheck turns one of a column's checks ('difficulty' or 'repeats') on or
        // off for its routine, overriding its requirement set's choice.
        setCheck(side, check, on) {
            const routine = this.routineFor(side);
            routine.checks = { ...routine.checks, [check]: on };
            this.persist();
            this.renderRoutines();
        },
        // startFromSetRoutine starts a routine from a set routine: the routine on
        // screen if it's empty, otherwise a new one named after the set. Its
        // requirements are left alone: the set is a starting point.
        async startFromSetRoutine(ref) {
            if (!ref) { return; }
            const skills = await this.fetchSetRoutine(ref);
            if (!skills) { this.showToast("Couldn't load that set routine.", 'error'); return; }
            const name = Exercises.requirementName(ref) !== ref ? Exercises.requirementName(ref) : (this.customSets.find((s) => s.id === ref)?.set.name || 'Set routine');
            if (this.routine.length === 0) {
                if (this.editingIndex !== null) { this.cancelEdit(); }
                this.expanded = skills.map(() => false);
                this.routine = skills; // re-renders and saves via the watcher
                this.showToast(`${this.currentRoutine().name} starts from ${name}.`, 'info');
                return;
            }
            const routine = { id: RoutineStore.newId(), name: name.slice(0, 60), skills };
            this.routines.splice(this.routines.indexOf(this.currentRoutine()) + 1, 0, routine);
            this.persist();
            this.switchRoutine(routine.id);
            this.showToast(`Started ${routine.name} from the set routine.`, 'info');
        },

        // --- Levels mode: a level's tabs, with its voluntaries as linked routines ---
        setMode(mode) {
            if (this.editingIndex !== null) { this.cancelEdit(); }
            this.mode = mode;
            try { localStorage.setItem('builderMode', mode); } catch (e) { /* remembered for this visit only */ }
            this.compareId = null;
            this.routineB = [];
            this.expandedB = [];
            this.mobileTab = 'a';
            this.addTo = '';
            if (mode === 'levels') { this.syncColumns(); } else { this.renderRoutines(); }
        },
        findLevel(ref) { return Exercises.findLevel(ref); },
        requirementName(ref) { return Exercises.requirementName(ref); },
        entry() { return this.levelState.entries.find((e) => e.id === this.levelState.current); },
        entryTabs() {
            this.customSets; this.customLevels; // re-evaluate when these change
            return this.entry() ? Exercises.tabs(this.findLevel(this.entry().level)) : [];
        },
        saveEntries() { LevelEntries.save(this.levelState); },
        // columnTab is the level tab a column shows in Levels mode: the open one,
        // or the one beside it.
        columnTab(side) {
            if (this.mode !== 'levels' || !this.entry()) { return undefined; }
            const key = side === 'b' ? this.entry().beside : this.entry().open;
            return key ? this.entryTabs().find((t) => t.key === key) : undefined;
        },
        // columnRoutine is the routine a column shows in Levels mode: the one
        // linked to its voluntary.
        columnRoutine(side) {
            const tab = this.columnTab(side);
            return tab && !tab.set ? LevelEntries.routineFor(this.entry(), tab.exercise, this.routines) : undefined;
        },
        twoColumns() { return this.mode === 'levels' ? !!this.columnTab('b') : !!this.compareId; },
        columnTitle(side) {
            const tab = this.columnTab(side);
            if (tab) { return tab.label; }
            const routine = this.routineFor(side);
            return routine ? `${routine.name} (${routine.skills.length})` : '';
        },
        isChosenTab(tab) {
            const entry = this.entry();
            return Exercises.options(this.findLevel(entry.level), tab.exercise).length > 1 && LevelEntries.chosen(entry, tab.exercise) === tab.ref;
        },
        // syncColumns shows the open and beside tabs' voluntary routines as the
        // current and compared routines, so they're edited like any other.
        syncColumns() {
            if (this.mode !== 'levels') { return; }
            const a = this.columnRoutine('a'), b = this.columnRoutine('b');
            if (a && a.id !== this.currentId) {
                if (this.editingIndex !== null) { this.cancelEdit(); }
                this.currentId = a.id;
                this.expanded = a.skills.map(() => false);
                this.routine = a.skills;
            }
            if (b) {
                if (this.editingSide === 'b' && this.editingIndex !== null) { this.cancelEdit(); }
                this.compareId = b.id;
                this.expandedB = b.skills.map(() => false);
                this.routineB = b.skills;
            } else {
                this.compareId = null;
                this.routineB = [];
                this.expandedB = [];
            }
            this.mobileTab = 'a'; // on a phone, the open tab
            this.persist();
            this.renderRoutines();
        },
        // newEntry starts working on a level, opening its first tab beside the
        // other exercise's (e.g. Set 1 beside the Voluntary).
        newEntry(ref) {
            const level = this.findLevel(ref);
            if (!level) { return; }
            const names = new Set(this.levelState.entries.map((e) => e.name));
            let name = level.name;
            for (let n = 2; names.has(name); n++) { name = `${level.name} (${n})`; }
            const entry = { id: LevelEntries.newId(), name, level: ref, exercises: [{}, {}] };
            const tabs = Exercises.tabs(level);
            entry.open = tabs[0]?.key;
            entry.beside = tabs.find((t) => t.exercise !== tabs[0]?.exercise)?.key || null;
            this.levelState.entries.push(entry);
            this.levelState.current = entry.id;
            this.saveEntries();
            this.syncColumns();
        },
        switchEntry(id) {
            this.levelState.current = id;
            this.saveEntries();
            this.syncColumns();
        },
        renameEntry() {
            const entry = this.entry();
            const name = entry && prompt('Name this level (e.g. the gymnast)', entry.name);
            if (!name || !name.trim()) { return; }
            entry.name = name.trim().slice(0, 60);
            this.saveEntries();
        },
        deleteEntry() {
            const entry = this.entry();
            if (!entry || !confirm(`Delete ${entry.name}? Its voluntary routines are kept under Routines.`)) { return; }
            this.levelState.entries = this.levelState.entries.filter((e) => e !== entry);
            this.levelState.current = this.levelState.entries[0]?.id || null;
            this.saveEntries();
            this.syncColumns();
        },
        // chooseOption makes a tab its exercise's choice (the set performed, or
        // the voluntary's requirements, which its routine is then checked against).
        chooseOption(tab) {
            const entry = this.entry();
            entry.exercises[tab.exercise - 1].option = tab.ref;
            if (!tab.set) {
                const routine = LevelEntries.routineFor(entry, tab.exercise, this.routines);
                if (routine && routine.requirements !== tab.ref) {
                    routine.requirements = tab.ref;
                    delete routine.checks;
                }
            }
        },
        openTab(tab) {
            const entry = this.entry();
            if (entry.open === tab.key) { return; }
            if (entry.beside === tab.key) { entry.beside = entry.open; }
            entry.open = tab.key;
            this.chooseOption(tab);
            this.saveEntries();
            this.syncColumns();
        },
        setBeside(key) {
            const entry = this.entry();
            entry.beside = key || null;
            const tab = this.entryTabs().find((t) => t.key === key);
            if (tab) { this.chooseOption(tab); }
            this.saveEntries();
            this.syncColumns();
        },
        // linkVoluntary sets the routine doing a column's voluntary: one already
        // built (checked against the voluntary's requirements from then on), a new
        // one ('new'), or none ('').
        linkVoluntary(side, id) {
            const tab = this.columnTab(side), entry = this.entry();
            if (!tab || tab.set) { return; }
            if (id === 'new') { this.newVoluntary(tab, []); return; }
            const routine = this.routines.find((r) => r.id === id);
            entry.exercises[tab.exercise - 1].routine = routine?.id;
            if (routine && routine.requirements !== tab.ref) {
                routine.requirements = tab.ref;
                delete routine.checks;
            }
            this.saveEntries();
            this.syncColumns();
        },
        // newVoluntary starts a routine for a voluntary tab, with skills (e.g. a
        // set's), and links it.
        newVoluntary(tab, skills) {
            const entry = this.entry();
            const routine = { id: RoutineStore.newId(), name: `${entry.name} · ${tab.label}`.slice(0, 60), skills, requirements: tab.ref };
            this.routines.push(routine);
            entry.exercises[tab.exercise - 1].routine = routine.id;
            this.saveEntries();
            this.syncColumns();
            this.showToast(`Started ${routine.name}.`, 'info');
        },
        // voluntaryFor is the voluntary tab a set routine column can start: the
        // other exercise's chosen one, or any voluntary.
        voluntaryFor(side) {
            const tab = this.columnTab(side);
            if (!tab?.set) { return undefined; }
            const voluntaries = this.entryTabs().filter((t) => !t.set);
            const other = voluntaries.filter((t) => t.exercise !== tab.exercise);
            const from = other.length > 0 ? other : voluntaries;
            return from.find((t) => LevelEntries.chosen(this.entry(), t.exercise) === t.ref) || from[0];
        },
        // startVoluntaryFrom starts a voluntary from a set routine: the set in a
        // column (into the voluntary it can start), or a given set (into the
        // voluntary in this column). Skills already there are replaced, if the
        // coach agrees, and the voluntary is shown.
        async startVoluntaryFrom(side, set = undefined) {
            const from = set || this.columnTab(side);
            const target = set ? this.columnTab(side) : this.voluntaryFor(side);
            if (!from?.set || !target || target.set) { return; }
            const skills = await this.fetchSetRoutine(from.ref);
            if (!skills) { this.showToast(`Couldn't load ${from.label}.`, 'error'); return; }
            const entry = this.entry();
            const routine = LevelEntries.routineFor(entry, target.exercise, this.routines);
            if (!routine) {
                this.newVoluntary(target, skills);
            } else {
                if (routine.skills.length > 0 && !confirm(`Replace the ${routine.skills.length} skills in ${routine.name} with ${from.label}?`)) { return; }
                routine.skills.splice(0, routine.skills.length, ...skills);
                this.persist();
                this.showToast(`${routine.name} starts from ${from.label}; change it from here.`, 'info');
            }
            if (entry.open !== target.key && entry.beside !== target.key) { entry.beside = target.key; }
            this.chooseOption(target);
            this.saveEntries();
            this.syncColumns();
        },
        viewHref() {
            if (this.mode === 'levels' && this.entry()) { return `/view?entry=${this.entry().id}`; }
            return `/view?routine=${this.currentId}`;
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
        // addTargets are the routines Add can put a skill in, for the "Add to" list.
        addTargets() { return this.routines.map((r) => ({ value: r.id, label: `${r.name} (${r.skills.length})` })); },
        // addToValue is the routine Add will put a skill in: the one chosen, or the
        // one on screen (in Levels mode, a voluntary shown or the level's first).
        addToValue() {
            if (this.addTo && this.routines.some((r) => r.id === this.addTo)) { return this.addTo; }
            if (this.mode === 'levels' && this.entry()) {
                const shown = this.columnRoutine('a') || this.columnRoutine('b');
                const linked = [1, 2].map((n) => LevelEntries.routineFor(this.entry(), n, this.routines)).find(Boolean);
                return (shown || linked)?.id || '';
            }
            return this.currentId;
        },
        // addTarget is the routine Add will put a skill in, and the column it's
        // shown in ('a' or 'b'), if it's on screen.
        addTarget() {
            const routine = this.routines.find((r) => r.id === this.addToValue());
            if (!routine) { return undefined; }
            let side;
            if (this.mode === 'levels') {
                side = this.columnRoutine('a')?.id === routine.id ? 'a' : this.columnRoutine('b')?.id === routine.id ? 'b' : undefined;
            } else {
                side = routine.id === this.currentId ? 'a' : routine.id === this.compareId ? 'b' : undefined;
            }
            return { routine, side, label: routine.name };
        },
        // addFromForm adds the skill shown on the card to the end of the routine
        // chosen under "Add to" (the one on screen unless another is chosen).
        async addFromForm() {
            if (this.busy) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                const target = this.addTarget();
                if (!target) { throw new Error('choose a routine to add to (in a level, a voluntary routine)'); }
                let skills;
                if (target.side) {
                    skills = this.skillsOf(target.side);
                    this.expandedOf(target.side).push(false);
                    skills.push(skill);
                    this.mobileTab = target.side; // show where it went
                } else {
                    skills = target.routine.skills;
                    skills.push(skill);
                    this.persist();
                    this.renderRoutines(); // a level panel counts the other exercise
                }
                const where = target.side === 'a' && !this.twoColumns() ? '' : ` to ${target.label}`;
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
            if (!list || !window.Sortable || 'readOnly' in list.dataset) { return; } // a prescribed set routine stays as it is
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
