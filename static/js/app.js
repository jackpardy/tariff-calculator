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
        shadeDiffs: true, // shade the second routine's skills that differ from the first's (kept in the browser)
        beside: null,    // in Routines mode, the routine side by side, remembered (so the view's back arrow, a reload or Levels and back keep it)
        routineB: [],    // its skills (the same array as in routines)
        expandedB: [],   // per-card expanded state, parallel to routineB
        addTo: '',       // the routine Add puts a skill in (its id); '' for the one on screen
        mobileTab: 'a',  // the column shown on a phone while comparing
        // A card being changed in place: the skill form is moved into it
        // (placeEditor), and back to Add a skill after.
        editingIndex: null,
        editingSide: 'a', // the column of the skill being edited
        // The skill as it will be, shown on the card's head while it's changed
        // (readDraft): {name, official, fig, tariff, changed}; null until the
        // card's form is in.
        draft: null,
        busy: false, // an add or update is in flight
        pickerTab: 'jumps', // the skill picker's open category
        picked: null,       // the picker entry last chosen (catalog.Entry ID)
        pickedShape: null,  // the shape it was chosen in
        query: '',          // the picker's search text; results replace the tabs while it's set
        toast: { show: false, message: '', type: 'info', undo: null },
        memberLinks: [], // club member pages saved in this browser, to keep routines on (ADR 0011)
        chooser: { open: '', q: '' }, // the open chooser ('check-a', 'check-b', 'set' or 'level') and its search
        recent: { check: [], set: [] }, // the built-in requirements and set routines chosen lately, latest first
        // ask is a question on the page, in place of the browser's prompt and
        // confirm: {title, text, value (a name to type, or null), buttons}.
        ask: { open: false, title: '', text: '', value: null, buttons: [] },
        writing: null, // requirements being written beside the routine: {id (the saved set, or null), title, draft (the last valid set's JSON)}

        init() {
            this.memberLinks = RoutineStore.memberLinks();
            try { this.recent = { check: [], set: [], ...JSON.parse(localStorage.getItem('recentRequirements') || '{}') }; } catch (e) { /* none yet */ }
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
            this.beside = state.beside && state.beside !== state.current && state.routines.some((r) => r.id === state.beside) ? state.beside : null;
            this.routine = this.currentRoutine().skills;
            this.persist(); // settles a routine carried over from an older version
            this.customSets = SetStore.load();
            this.customLevels = LevelStore.load();
            // Requirements kept on a person's page (ADR 0013), read in.
            KeptRequirements.sync().then((changed) => {
                if (changed) { this.customSets = SetStore.load(); this.customLevels = LevelStore.load(); }
            });
            this.levelState = LevelEntries.load();
            try { this.mode = localStorage.getItem('builderMode') === 'levels' ? 'levels' : 'routines'; } catch (e) { /* routines */ }
            try { this.shadeDiffs = localStorage.getItem('builderShadeDiffs') !== 'off'; } catch (e) { /* shaded */ }
            this.$watch('shadeDiffs', (on) => { try { localStorage.setItem('builderShadeDiffs', on ? 'on' : 'off'); } catch (e) { /* this visit only */ } });
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
                if (this.writing) { this.routineChangedWhileWriting(); }
            });
            this.$watch('routineB', (routine) => {
                if (!this.compareId) { return; }
                this.compareRoutine().skills = routine;
                this.renderRoutines();
                this.persist();
            });

            // A routine's view is redrawn whole: a card's skill form steps out
            // first, and goes back into the card in the new one.
            document.body.addEventListener('htmx:beforeSwap', (event) => {
                const id = event.detail.target.id;
                if (id === 'routine-view' || id === 'routine-view-b') { this.parkEditor(event.detail.target); }
            });
            document.body.addEventListener('htmx:afterSwap', (event) => {
                const id = event.detail.target.id;
                if (id === 'routine-view') { this.makeSortable('a'); }
                if (id === 'routine-view-b') { this.makeSortable('b'); }
                if (id === 'routine-view' || id === 'routine-view-b') {
                    this.placeEditor();
                    // Side by side both columns are redrawn: the Edit is focused in its own.
                    const edit = this._refocusEdit && event.detail.target.querySelector(this._refocusEdit);
                    if (edit) { edit.focus({ preventScroll: true }); this._refocusEdit = null; }
                }
                // After Enter in a card's search, the typing's own search may come
                // back later and replace the results: the focus goes back on the first.
                if (id === 'search-results' && this._enterFor !== null && this._enterFor === this.query) { this.focusFirstResult(); }
                // A card being changed shows the skill on its head as it changes.
                if (id === 'skill-editor' || id === 'skill-form-wrapper') { this.readDraft(); }
                // A result chosen in a card being changed: on to Update, on its head.
                if (id === 'skill-editor' && this._toUpdate) {
                    this._toUpdate = false;
                    this.$nextTick(() => document.getElementById('update-btn')?.focus({ preventScroll: true }));
                }
            });
            this.holdOnResize();
            document.body.addEventListener('htmx:afterRequest', (event) => {
                if (event.detail.failed) { this.requestFailed(event.detail.requestConfig?.path, event.detail.xhr); }
            });
            // Requirements written beside the routine: it's checked against them as they change.
            document.body.addEventListener('htmx:afterSettle', (event) => {
                if (this.writing && event.detail.requestConfig?.path === '/requirements/editor') { this.draftChanged(); }
            });

            if (this.mode === 'levels') { this.syncColumns(); } else if (this.beside) { this.compareWith(this.beside); } else { this.renderRoutines(); }
            this.loadForm();
            this.openFromLink();
        },
        // openFromLink opens a set routine the requirements page sent here:
        // ?setRoutine=<id> to edit one of the coach's (saved back with "Save as
        // a set routine"), ?fromSet=<ref> to start a routine from a copy.
        async openFromLink() {
            const params = new URLSearchParams(location.search);
            const ref = params.get('setRoutine') || params.get('fromSet');
            if (!ref) { return; }
            history.replaceState(null, '', location.pathname);
            const skills = await this.fetchSetRoutine(ref);
            if (!skills) { this.showToast("Couldn't open that set routine.", 'error'); return; }
            const editing = params.has('setRoutine');
            const name = this.customSets.find((s) => s.id === ref)?.set.name || Exercises.requirementName(ref);
            // Editing it again goes back to the routine it's being edited in,
            // as it was left (changes not saved yet stay), rather than adding another.
            const open = editing && this.routines.find((r) => r.setRoutine === ref);
            if (open) {
                if (this.mode !== 'routines') { this.setMode('routines'); }
                this.switchRoutine(open.id);
                this.showToast(`Editing ${name}, as you left it here: change it, then More → Save as a set routine.`, 'info');
                return;
            }
            const routine = { id: RoutineStore.newId(), name: (editing ? name : `Copy of ${name}`).slice(0, 60), skills };
            if (editing) { routine.setRoutine = ref; }
            this.routines.push(routine);
            this.persist();
            if (this.mode !== 'routines') { this.setMode('routines'); }
            this.switchRoutine(routine.id);
            this.showToast(editing ? `Editing ${name}: change it, then More → Save as a set routine.` : `Started ${routine.name}: change it, then More → Save as a set routine.`, 'info');
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
                    // Only a level with set routines can start the voluntary from one.
                    const fromSet = this.entryTabs().some((t) => t.set) ? ', or start from a set' : '';
                    if (p) { this.parkEditor(p); p.innerHTML = `<p class="has-text-grey level-empty">No routine for this voluntary yet. Choose one above: a new routine or one you've built${fromSet}.</p>`; }
                    return;
                }
                const entryValues = LevelEntries.values(this.entry(), tab.exercise, this.routines);
                values = tab.set
                    ? { side, prescribed: '1', requirementSet: SetStore.payload(tab.ref), ...entryValues }
                    : { side, ...Exercises.values(routine), ...entryValues };
            } else {
                values = { side, ...Exercises.values(this.routineFor(side)) };
                // Requirements being written beside it: checked against the draft.
                if (side === 'a' && this.writing?.draft) { values.requirementSet = this.writing.draft; }
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
                    this.parkEditor(view);
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
        persist() { RoutineStore.save({ current: this.currentId, routines: this.routines, beside: this.beside || undefined }); },
        // setRequirements chooses the requirements a column's routine is checked against.
        setRequirements(ref, side = 'a') {
            const routine = this.routineFor(side);
            routine.requirements = ref || undefined;
            delete routine.checks; // the new requirements' checks apply
            this.persist();
            this.renderRoutine(side);
        },
        // --- Writing requirements beside the routine (views.requirementsWriter) ---
        isYours(ref) { return !!ref && !ref.startsWith('builtin:') && this.customSets.some((s) => s.id === ref); },
        // writeRequirements opens the editor beside the routine: on one of
        // your own requirements (ref), a copy of a built-in one, or new ('').
        writeRequirements(ref) {
            if (this.mode !== 'routines') { return; }
            const saved = this.customSets.find((s) => s.id === ref);
            let values;
            if (saved) {
                this.writing = { id: saved.id, title: `Change ${saved.set.name}`, draft: JSON.stringify(saved.set) };
                values = { set: JSON.stringify(saved.set) };
            } else if (ref.startsWith('builtin:')) {
                this.writing = { id: null, title: `A copy of ${Exercises.requirementName(ref)}`, draft: SetStore.payload(ref) };
                values = { copy: ref };
            } else {
                this.writing = { id: null, title: 'Your own requirements', draft: '' };
                values = { set: JSON.stringify({ format: 1, name: '', rules: [] }) };
            }
            // The routine goes with it, for each rule to say whether it's met.
            values.routineData = JSON.stringify(this.routine);
            htmx.ajax('POST', '/requirements/editor', { source: '#requirements-writer-editor', target: '#requirements-writer-editor', swap: 'innerHTML', values })
                .then(() => {
                    this.draftChanged();
                    document.querySelector('.requirements-writer')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
                });
        },
        // draftChanged checks the routine against the requirements as the
        // editor has them now, if they can be read (no problems listed).
        draftChanged() {
            const form = document.querySelector('#requirements-writer-editor #set-editor');
            if (!form || !this.writing) { return; }
            if (form.dataset.problems === '0') { this.writing.draft = form.dataset.setJson; }
            this.renderRoutine('a');
        },
        // saveWriting saves the requirements written beside the routine (as
        // the Requirements page saves them, and on your page if they're kept
        // there), and checks the routine against them.
        async saveWriting() {
            const form = document.querySelector('#requirements-writer-editor #set-editor');
            if (!form || !this.writing) { return; }
            const response = await fetch('/requirements/editor', { method: 'POST', body: saveBody(form) });
            if (!response.ok) { this.showToast(`Couldn't save: ${(await response.text()).trim()}`, 'error'); return; }
            const html = await response.text();
            swapIn(form, html);
            const fresh = document.querySelector('#requirements-writer-editor #set-editor');
            const set = JSON.parse(fresh.dataset.setJson);
            if (fresh.dataset.problems !== '0') { this.showToast('Fix the problems listed at the top first.', 'error'); fresh.scrollIntoView({ block: 'start' }); return; }
            if (!set.name?.trim()) { this.showToast('Give the requirements a name first.', 'error'); fresh.querySelector('[name=name]')?.focus(); return; }
            const sets = SetStore.load();
            let saved = sets.find((s) => s.id === this.writing.id);
            const described = JSON.parse(fresh.dataset.described || '[]');
            if (saved) {
                Object.assign(saved, { set, described });
            } else {
                saved = { id: SetStore.newId(), set, described };
                sets.push(saved);
            }
            SetStore.save(sets);
            this.customSets = sets;
            this.writing = null;
            this.setRequirements(saved.id, 'a');
            if (saved.kept) {
                try { await KeptRequirements.save(saved.kept, 'set', saved); } catch (e) {
                    this.showToast(`Saved here, but not on your page (${e.message}): try again when there's a signal.`, 'error');
                    return;
                }
            }
            this.showToast(`Saved ${set.name}. This routine is checked against it.`, 'info');
        },
        // routineChangedWhileWriting posts the editor again with the routine as
        // it is now, so each rule says whether it's met.
        routineChangedWhileWriting() {
            const form = document.querySelector('#requirements-writer-editor #set-editor');
            const field = form?.querySelector('[name=routineData]');
            if (!field) { return; }
            field.value = JSON.stringify(this.routine);
            htmx.trigger(form, 'change');
        },
        cancelWriting() {
            this.writing = null;
            document.getElementById('requirements-writer-editor')?.replaceChildren();
            this.renderRoutine('a');
        },

        // --- Choosers: Check against and Start from a set (views/chooser.templ) ---
        toggleChooser(key) {
            this.chooser = { open: this.chooser.open === key ? '' : key, q: '' };
            if (this.chooser.open) {
                this.$nextTick(() => document.querySelector('.chooser-panel:not([style*=none]) .chooser-search')?.focus());
            }
        },
        closeChooser(key) {
            if (this.chooser.open === key) { this.chooser = { open: '', q: '' }; }
        },
        // chooserShows reports whether an option (with its search text in
        // data-search) matches the search: every word of it, ignoring case and accents.
        chooserShows(el) {
            // Accents and case aside, any dash is a hyphen ("13-14" finds "13–14"),
            // and "level 3" is "l3", as the levels are named.
            const plain = (text) => text.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[\u2010-\u2015]/g, '-');
            const text = plain(el.dataset.search || el.textContent);
            const query = plain(this.chooser.q).replace(/\blevel\s*(\d)/g, 'l$1');
            return query.split(/\s+/).every((word) => text.includes(word));
        },
        // chooserAny reports whether any option inside el matches the search.
        chooserAny(el) {
            this.chooser.q; // re-evaluate as it changes
            return [...el.querySelectorAll('[data-search]')].some((o) => this.chooserShows(o));
        },
        chosenRequirements(side) { return this.routineFor(side)?.requirements || ''; },
        checkLabel(side) {
            const ref = this.chosenRequirements(side);
            if (!ref) { return 'No requirements'; }
            return this.customSets.find((s) => s.id === ref)?.set.name || Exercises.requirementName(ref);
        },
        // yourChoices are your own requirements, set routines or levels (kind
        // 'check', 'set' or 'level'): those kept on your page first.
        yourChoices(kind) {
            if (kind === 'level') {
                return this.customLevels
                    .map((l) => ({ ref: l.id, name: l.level.name, kept: !!l.kept, search: `${l.level.name} ${l.kept ? 'on your page' : ''}` }))
                    .sort((a, b) => Number(b.kept) - Number(a.kept));
            }
            return this.customSets
                .filter((s) => this.isSetRoutine(s.set) === (kind === 'set') && !s.inLevel) // a level's own are its
                .map((s) => ({ ref: s.id, name: s.set.name, kept: !!s.kept, search: `${s.set.name} ${s.kept ? 'on your page' : ''}` }))
                .sort((a, b) => Number(b.kept) - Number(a.kept));
        },
        // recentChoices are the built-in ones chosen lately, still there.
        recentChoices(kind) {
            const names = Exercises.data().names || {};
            return (this.recent[kind] || []).filter((ref) => names[ref]).map((ref) => ({ ref, name: names[ref] }));
        },
        // choose takes a chooser's option: requirements for its column to be
        // checked against, a set routine to start from, or a level to start.
        choose(key, ref) {
            this.chooser = { open: '', q: '' };
            if (key === 'level') { this.newEntry(ref); return; }
            const kind = key === 'set' ? 'set' : 'check';
            if (ref.startsWith('builtin:')) {
                this.recent[kind] = [ref, ...(this.recent[kind] || []).filter((r) => r !== ref)].slice(0, 4);
                try { localStorage.setItem('recentRequirements', JSON.stringify(this.recent)); } catch (e) { /* remembered for this visit only */ }
            }
            if (kind === 'set') {
                this.startFromSetRoutine(ref);
            } else {
                this.setRequirements(ref, key === 'check-b' ? 'b' : 'a');
            }
        },
        // setCheck turns one of a column's checks ('difficulty' or 'repeats') on or
        // off for its routine, overriding its requirement set's choice.
        setCheck(side, check, on) {
            const routine = this.routineFor(side);
            routine.checks = { ...routine.checks, [check]: on };
            this.persist();
            this.renderRoutines();
        },
        // saveToPage keeps the current routine on a club member's page (one of
        // memberLinks), which then opens; keeping it again changes it there.
        saveToPage(member) {
            this.persist();
            RoutineStore.keep(member.url, [this.currentRoutine()]);
        },
        // saveAsSetRoutine saves the current routine as a set routine (one of the
        // coach's requirements): exactly its skills, scored like a set routine. A
        // routine saved before (or opened from a set routine) can update it.
        async saveAsSetRoutine() {
            const routine = this.currentRoutine();
            if (routine.skills.length === 0) { this.showToast('Add the set routine\'s skills first.', 'error'); return; }
            const sets = SetStore.load();
            let existing = sets.find((s) => s.id === routine.setRoutine);
            if (existing) {
                const choice = await this.askUser({
                    title: `Save as the set routine ${existing.set.name}`,
                    text: 'Change that set routine to this routine, or save this as a new one?',
                    buttons: [{ label: `Update ${existing.set.name}`, value: 'update', primary: true }, { label: 'Save as a new one', value: 'new' }],
                });
                if (choice === null) { return; }
                if (choice === 'new') { existing = undefined; }
            }
            const name = existing ? existing.set.name : await this.askName('Name the set routine', routine.name);
            if (!name) { return; }
            const body = new URLSearchParams({ routineData: JSON.stringify(routine.skills), name, source: existing?.set.source || '' });
            const response = await fetch('/requirements/set-routine', { method: 'POST', body });
            if (!response.ok) { this.showToast(`Couldn't save it: ${(await response.text()).trim()}`, 'error'); return; }
            const set = await response.json();
            if (existing?.set.description) { set.description = existing.set.description; }
            if (existing) {
                existing.set = set;
            } else {
                existing = { id: SetStore.newId(), set };
                sets.push(existing);
            }
            SetStore.save(sets);
            this.customSets = sets;
            routine.setRoutine = existing.id;
            this.persist();
            this.showToast(`Saved the set routine ${name}. Use it in a level, or start routines from it.`, 'info');
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
            if (this.writing) { this.cancelWriting(); }
            this.mode = mode;
            try { localStorage.setItem('builderMode', mode); } catch (e) { /* remembered for this visit only */ }
            this.compareId = null;
            this.routineB = [];
            this.expandedB = [];
            this.mobileTab = 'a';
            this.addTo = '';
            if (mode === 'levels') {
                this.syncColumns();
            } else if (this.beside && this.beside !== this.currentId && this.routines.some((r) => r.id === this.beside)) {
                this.compareWith(this.beside);
            } else {
                this.beside = null;
                this.renderRoutines();
            }
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
            return routine ? RoutineStore.label(routine) : '';
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
        async renameEntry() {
            const entry = this.entry();
            const name = entry && await this.askName('Name this level (e.g. the gymnast)', entry.name);
            if (!name) { return; }
            entry.name = name.slice(0, 60);
            this.saveEntries();
        },
        async deleteEntry() {
            const entry = this.entry();
            if (!entry || !await this.askSure(`Delete ${entry.name}? Its voluntary routines are kept under Routines.`)) { return; }
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
            // Beside the open tab, another of the same exercise (Set 2 beside
            // Set 1) is only to compare: the exercise's choice stays the open one.
            const tab = this.entryTabs().find((t) => t.key === key);
            const open = this.entryTabs().find((t) => t.key === entry.open);
            if (tab && tab.exercise !== open?.exercise) { this.chooseOption(tab); }
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
                const was = routine.requirements;
                routine.requirements = tab.ref;
                delete routine.checks;
                // Say so: Routines mode now checks it against the voluntary's requirements.
                const from = was ? ` instead of ${Exercises.requirementName(was) || 'what it was'}` : '';
                this.showToast(`${routine.name} is now checked against ${Exercises.requirementName(tab.ref) || tab.label}${from}.`, 'info');
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
                if (routine.skills.length > 0 && !await this.askSure(`Replace the ${routine.skills.length} skills in ${routine.name} with ${from.label}?`, 'Replace them')) { return; }
                routine.skills.splice(0, routine.skills.length, ...skills);
                this.persist();
                this.showToast(`${routine.name} starts from ${from.label}; change it from here.`, 'info');
            }
            // On a phone, where there's no column beside, the voluntary opens.
            if (window.matchMedia('(max-width: 768px)').matches) {
                if (entry.beside === target.key) { entry.beside = entry.open; }
                entry.open = target.key;
            } else if (entry.open !== target.key && entry.beside !== target.key) {
                entry.beside = target.key;
            }
            this.chooseOption(target);
            this.saveEntries();
            this.syncColumns();
        },
        // viewHref opens the view screen on what's on screen: a level, two
        // routines side by side, or one routine.
        viewHref() {
            if (this.mode === 'levels' && this.entry()) { return `/view?entry=${this.entry().id}`; }
            if (this.compareId) { return `/view?routine=${this.currentId}&beside=${this.compareId}`; }
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
            const copy = { ...JSON.parse(JSON.stringify(source)), id: RoutineStore.newId(), name: `Copy of ${source.name}`.slice(0, 60) };
            delete copy.setRoutine; // a copy saves as a set routine of its own
            this.routines.splice(this.routines.indexOf(source) + 1, 0, copy);
            this.switchRoutine(copy.id);
            this.showToast(`Duplicated as ${copy.name}.`, 'info');
        },
        async renameRoutine() {
            const routine = this.currentRoutine();
            const name = await this.askName('Name this routine', routine.name);
            if (!name) { return; }
            routine.name = name.slice(0, 60);
            this.persist();
        },
        // --- Side by side ---
        // compareWith shows the routine with id beside the current one ('' stops).
        compareWith(id) {
            if (!id || id === this.currentId || !this.routines.some((r) => r.id === id)) { this.stopComparing(); return; }
            if (this.editingSide === 'b' && this.editingIndex !== null) { this.cancelEdit(); }
            this.compareId = id;
            if (this.mode === 'routines') { this.beside = id; }
            this.expandedB = this.compareRoutine().skills.map(() => false);
            this.routineB = this.compareRoutine().skills; // renders both columns via the watcher
            this.persist();
        },
        stopComparing() {
            if (this.editingSide === 'b' && this.editingIndex !== null) { this.cancelEdit(); }
            this.compareId = null;
            if (this.mode === 'routines') { this.beside = null; }
            this.routineB = [];
            this.expandedB = [];
            this.mobileTab = 'a';
            this.persist();
            this.renderRoutine('a');
        },
        // swapColumns makes the routine beside the current one current, and vice versa.
        swapColumns() {
            if (!this.compareId) { return; }
            [this.currentId, this.compareId] = [this.compareId, this.currentId];
            if (this.mode === 'routines') { this.beside = this.compareId; }
            [this.expanded, this.expandedB] = [this.expandedB, this.expanded];
            const flip = (side) => (side === 'a' ? 'b' : 'a');
            this.editingSide = flip(this.editingSide);
            this.mobileTab = flip(this.mobileTab); // keep showing the same routine
            this.routine = this.currentRoutine().skills;
            this.routineB = this.compareRoutine().skills;
        },

        async deleteRoutine() {
            const routine = this.currentRoutine(), name = routine.name;
            if (!await this.askSure(`Delete ${name}? This can't be undone.`)) { return; }
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
        // Both columns, when side by side.
        expandAll() { this.expanded = this.routine.map(() => true); this.expandedB = this.routineB.map(() => true); },
        collapseAll() { this.expanded = this.routine.map(() => false); this.expandedB = this.routineB.map(() => false); },

        // --- Adding and editing skills ---
        // calculateFormSkill posts the form and returns the skill as the routine stores it.
        async calculateFormSkill() {
            const form = document.getElementById('main-form');
            const response = await fetch('/calculate-skill', { method: 'POST', body: new URLSearchParams(new FormData(form)) });
            if (!response.ok) { throw new Error((await response.text()).trim() || `HTTP ${response.status}`); }
            return response.json();
        },
        // addTargets are the routines Add can put a skill in, for the "Add to" list.
        addTargets() { return this.routines.map((r) => ({ value: r.id, label: RoutineStore.label(r) })); },
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
        // previousSkill is the skill before where the card's skill goes, as JSON,
        // for a body-landing tile to take off where it landed: the one before the
        // skill being edited, or the last of the routine Add puts it in; '' at the
        // start of a routine.
        previousSkill() {
            const skills = this.editingIndex !== null ? this.skillsOf(this.editingSide).slice(0, this.editingIndex) : this.addTarget()?.routine.skills || [];
            const prev = skills[skills.length - 1];
            return prev ? JSON.stringify(prev) : '';
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
                // One message: past ten, the note goes with what was added.
                const over = skills.length > 10 ? ` Note: an exercise has 10 skills, and this has ${skills.length}.` : '';
                this.showToast(`Added ${skill.custom_name || skill.name} (${skill.tariff.toFixed(1)})${where}.${over}`, over ? 'warning' : 'info');
                this.clearLabel(); // a label belongs to one skill
                this.query = '';   // back to the picker for the next skill
            } catch (error) {
                this.showToast(`Can't add skill: ${error.message}`, 'error');
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

        // --- Changing a skill in its card ---
        // editSkill opens a card into the skill form, to change the skill in
        // place: the form moves out of Add a skill into the card, under its
        // head, once it's loaded with the skill, and the head shows the skill
        // as it changes (readDraft), with Update and Cancel. Edit on another
        // card moves it there (the first left as it was); Edit again on the
        // same one closes it.
        editSkill(index, side = 'a') {
            if (this.editingIndex === index && this.editingSide === side) { this.cancelEdit(true); return; }
            this.holdPlace(() => this.cardAt(index, side)?.querySelector('.card-head'));
            this.editingIndex = index;
            this.editingSide = side;
            this.draft = null; // the card shows its skill until its form is in
            this.query = '';
            this._enterFor = null;
            this._toUpdate = false;
            this._refocusEdit = null;
            this.mobileTab = side;
            this.loadForm(index, side)
                .then(() => {
                    this.placeEditor();
                    this.keepHeld();
                    this.$nextTick(() => {
                        this.showEditor();
                        if (this.wide()) { document.getElementById('skill-search')?.focus({ preventScroll: true }); }
                    });
                })
                .catch(() => { this.showToast('Failed to load the form.', 'error'); this.cancelEdit(); });
        },
        async updateFromForm() {
            const index = this.editingIndex, side = this.editingSide, skills = this.skillsOf(side);
            if (this.busy || index === null || index >= skills.length) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                skills.splice(index, 1, skill);
                // The routine is redrawn: the focus goes back on the card's Edit once it is.
                this._refocusEdit = `[data-edit="${side}:${index}"]`;
                this.cancelEdit(true);
            } catch (error) {
                this.showToast(`Can't update skill: ${error.message}`, 'error');
            } finally {
                this.busy = false;
            }
        },
        // cancelEdit stops changing a card (Cancel, Escape, Update, or the
        // skill or its routine gone): the form goes back to Add a skill, fresh
        // for adding, the card staying where it is on the screen, and (with
        // refocus) the focus back on the card's Edit.
        cancelEdit(refocus = false) {
            if (this.editingIndex === null) { return Promise.resolve(); }
            const index = this.editingIndex, side = this.editingSide;
            this.holdPlace(() => this.cardAt(index, side)?.querySelector('.card-head'));
            this.editingIndex = null;
            this.editingSide = 'a';
            this.draft = null;
            this.query = '';
            this._enterFor = null;
            this._toUpdate = false;
            document.querySelector('.builder-add').style.minHeight = '';
            this.placeEditor();
            this.keepHeld();
            if (refocus) { this.$nextTick(() => document.querySelector(`[data-edit="${side}:${index}"]`)?.focus({ preventScroll: true })); }
            // Held until the fresh form is in and shown; only this hold is let
            // go, not one Edit has made since (a quick Edit after Update).
            const hold = this._hold;
            return this.loadForm()
                .catch(() => this.showToast('Failed to load the form.', 'error'))
                .finally(() => requestAnimationFrame(() => requestAnimationFrame(() => { if (this._hold === hold) { this.releaseHold(); } })));
        },
        // escapeEdit is Escape in a card's form: it closes it, as Cancel does,
        // unless it's clearing what's typed in the search.
        escapeEdit(event) {
            if (this.editingIndex === null || (event.target.id === 'skill-search' && event.target.value)) { return; }
            this.cancelEdit(true);
        },
        // drafting reports whether the card at index in a column is being
        // changed, its form in: its head shows the skill as it will be, with
        // Update and Cancel in place of ↑, ↓, Edit and ✕.
        drafting(index, side) { return this.draft !== null && this.editingIndex === index && this.editingSide === side; },
        // readDraft reads the skill being built off the form, as the server
        // last drew it (its name, notation and tariff) and the label as it's
        // typed, for the card's head; "not saved" once it differs from what
        // the head showed.
        readDraft() {
            const form = document.getElementById('main-form');
            if (this.editingIndex === null || !form || !(Number(form.elements.editIndex?.value) >= 0)) { return; }
            const text = (sel) => form.querySelector(sel)?.textContent.trim() || '';
            const official = text('.skill-card-name'), label = document.getElementById('custom-name')?.value.trim() || '';
            const draft = { name: label || official, official: label ? official : '', fig: text('.skill-card-fig'), tariff: text('.skill-card-tariff') };
            const saved = this.cardAt(this.editingIndex, this.editingSide)?.querySelector('.card-saved');
            const flat = (s) => s.replace(/\s+/g, '');
            const shown = `${this.editingIndex + 1}.${draft.name}${draft.official ? '·' + draft.official : ''}`;
            draft.changed = !!saved && (flat(saved.querySelector('.skill-name').textContent) !== flat(shown)
                || flat(saved.querySelector('.card-fig').textContent) !== flat(draft.fig)
                || saved.closest('.card-toggle').querySelector('.card-tariff').textContent.trim() !== draft.tariff);
            this.draft = draft;
        },
        // changingWhere is what the Add a skill box says while a card is being changed.
        changingWhere() {
            if (this.editingIndex === null) { return ''; }
            const routine = this.mode === 'levels' ? this.columnRoutine(this.editingSide) : this.routineFor(this.editingSide);
            const of = this.twoColumns() && routine ? ` of ${routine.name}` : '';
            return `Changing skill ${this.editingIndex + 1}${of} in its card below: Update or Cancel there, then carry on adding.`;
        },
        // searchEnter is Enter in the search. Adding, it puts the first result
        // of what's typed now on the card (not of the last search to come
        // back). In a card being changed, it puts the focus on the first
        // result, so Enter again puts that on the card, the focus moving on to
        // Update, and a third Enter updates.
        async searchEnter(input) {
            const q = input.value;
            if (this.editingIndex === null) {
                await htmx.ajax('GET', '/skill-search?q=' + encodeURIComponent(q), { target: '#search-results', swap: 'innerHTML' });
                document.querySelector('#search-results .picker-skill')?.click();
                return;
            }
            if (!q.trim()) { return; }
            this._enterFor = q;
            await htmx.ajax('GET', '/skill-search?q=' + encodeURIComponent(q), { target: '#search-results', swap: 'innerHTML' });
            this.focusFirstResult();
        },
        _enterFor: null,  // the search Enter was pressed on in a card, while its first result has the focus
        _toUpdate: false, // a result is on its way to a card's form: the focus goes on to Update
        focusFirstResult() { document.querySelector('#search-results .picker-skill')?.focus({ preventScroll: true }); },
        // resultChosen notes a search result chosen in a card being changed:
        // the results close, and once it's on the card's head, the focus is
        // on Update there.
        resultChosen(event) {
            if (this.editingIndex === null || !event.target.closest('.picker-skill')) { return; }
            this._enterFor = null;
            this._toUpdate = true;
            this.query = '';
        },
        // wide reports a laptop or wider with a mouse, where typing is at hand:
        // there a card's form opens with the cursor in its search.
        wide() { return window.matchMedia('(min-width: 1024px) and (hover: hover)').matches; },
        // cardAt is the card of the skill at index in a column.
        cardAt(index, side) {
            return document.getElementById(side === 'b' ? 'routine-view-b' : 'routine-view')?.querySelectorAll('.routine-skill-container')[index];
        },
        // editorHome is where the skill form belongs: in the card being
        // changed (parked while that card is redrawn), otherwise in Add a skill.
        editorHome() {
            if (this.editingIndex === null) { return document.querySelector('.builder-add'); }
            return this.cardAt(this.editingIndex, this.editingSide) || document.getElementById('editor-park');
        },
        // placeEditor moves the skill form to its home, keeping the cursor
        // where it was.
        placeEditor() {
            const wrapper = document.getElementById('skill-form-wrapper');
            const home = this.editorHome();
            if (!wrapper || !home || wrapper.parentElement === home) { return; }
            const focus = this._refocus || this.focusIn(wrapper);
            this._refocus = null;
            home.appendChild(wrapper);
            if (focus?.el.isConnected && wrapper.contains(focus.el)) {
                focus.el.focus({ preventScroll: true });
                if (focus.start !== null) { try { focus.el.setSelectionRange(focus.start, focus.end); } catch (e) { /* not a text field */ } }
            }
        },
        // parkEditor takes the skill form out of a view about to be redrawn.
        parkEditor(view) {
            const wrapper = document.getElementById('skill-form-wrapper');
            if (!wrapper || !view.contains(wrapper)) { return; }
            this._refocus = this.focusIn(wrapper);
            document.getElementById('editor-park').appendChild(wrapper);
        },
        // focusIn is the field focused inside el, with its selection, if any.
        focusIn(el) {
            const active = document.activeElement;
            if (!active || !el.contains(active)) { return null; }
            let start = null, end = null;
            try { start = active.selectionStart; end = active.selectionEnd; } catch (e) { /* not a text field */ }
            return { el: active, start, end };
        },
        // showEditor brings a card's form into view once it's opened, gently
        // and only as far as it must: as much of it as fits, the card's head
        // kept in sight. The card is let go of here.
        showEditor() {
            this.releaseHold();
            const wrapper = document.getElementById('skill-form-wrapper');
            const card = wrapper?.closest('.routine-skill-container');
            if (!card) { return; }
            const margin = 16;
            const below = wrapper.getBoundingClientRect().bottom + margin - window.innerHeight;
            const by = Math.min(below, card.getBoundingClientRect().top - margin);
            if (by <= 0) { return; }
            const smooth = !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
            window.scrollBy({ top: by, behavior: smooth ? 'smooth' : 'instant' });
        },

        // --- Nothing moves under the finger ---
        // The form moving out of Add a skill (or back) changes that box's
        // height, above the routine. Meanwhile the card being changed is held
        // where it is on the screen: the page scrolls by as much as the card
        // would move, and where it can't (at the top of the page), the box
        // keeps the room until the form comes back. The browser's own scroll
        // anchoring is off meanwhile, so the two don't both make up for it.
        // holdPlace holds what at gives (a function, as the routine may be
        // redrawn meanwhile) where it is now.
        holdPlace(at) {
            const el = at();
            this._hold = el ? { at, top: el.getBoundingClientRect().top, y: window.scrollY } : null;
            document.documentElement.style.overflowAnchor = this._hold ? 'none' : '';
        },
        releaseHold() {
            this._hold = null;
            document.documentElement.style.overflowAnchor = '';
        },
        // keepHeld puts what's held back where it was.
        keepHeld() {
            const hold = this._hold, el = hold?.at();
            if (!el) { return; }
            const drift = el.getBoundingClientRect().top - hold.top;
            if (Math.abs(drift) < 1) { return; }
            window.scrollBy({ top: drift, behavior: 'instant' });
            const left = el.getBoundingClientRect().top - hold.top;
            if (left < -1) {
                const box = document.querySelector('.builder-add');
                box.style.minHeight = `${box.offsetHeight - left}px`;
            } else if (left > 1) {
                // At the foot of the page: room under the builder to scroll into,
                // taken away again once it's out of sight (holdOnResize).
                this._room = (this._room || 0) + left;
                document.querySelector('.builder').style.paddingBottom = `${this._room}px`;
                window.scrollBy({ top: left, behavior: 'instant' });
            }
            hold.y = window.scrollY;
        },
        // holdOnResize keeps the hold as the box changes height (Alpine and
        // htmx change it in their own time), and follows the page as it's
        // scrolled by hand meanwhile, so it never pulls the page back. Room
        // made at the foot of the page goes once it's scrolled out of sight,
        // so nothing jumps.
        holdOnResize() {
            const box = document.querySelector('.builder-add');
            if (box && window.ResizeObserver) { new ResizeObserver(() => this.keepHeld()).observe(box); }
            // Only a scroll by hand is followed: the browser pulls the page back
            // by itself when it gets shorter (the box shrinking a moment before
            // the form lands in the card), and following that held the card in
            // the wrong place.
            let byHand = 0;
            const hand = () => { byHand = performance.now(); };
            const scrollKeys = ['PageUp', 'PageDown', 'Home', 'End', 'ArrowUp', 'ArrowDown', ' '];
            window.addEventListener('wheel', hand, { passive: true, capture: true });
            window.addEventListener('touchmove', hand, { passive: true, capture: true });
            // A press on the page's scrollbar (its target is the page itself), not a click on a button.
            window.addEventListener('pointerdown', (e) => { if (e.target === document.documentElement) { hand(); } }, { passive: true, capture: true });
            window.addEventListener('keydown', (e) => { if (scrollKeys.includes(e.key) && !e.target.closest?.('input, textarea, select, [contenteditable]')) { hand(); } }, { passive: true, capture: true });
            window.addEventListener('scroll', () => {
                const hold = this._hold;
                if (hold && window.scrollY !== hold.y && performance.now() - byHand < 1000) {
                    hold.top -= window.scrollY - hold.y;
                    hold.y = window.scrollY;
                }
                if (this._room && !hold && window.scrollY + window.innerHeight <= document.documentElement.scrollHeight - this._room) {
                    this._room = 0;
                    document.querySelector('.builder').style.paddingBottom = '';
                }
            }, { passive: true });
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
        // removeSkill takes a skill out of its routine, with Undo on the
        // message, in place of asking first.
        removeSkill(index, side = 'a') {
            const skills = this.skillsOf(side);
            if (index < 0 || index >= skills.length) { return; }
            this.expandedOf(side).splice(index, 1);
            const [skill] = skills.splice(index, 1);
            this.showToast(`Removed ${skill.custom_name || skill.name}.`, 'info', () => this.restoreSkill(skills, index, skill, side));
            if (this.editingSide !== side) { return; }
            if (this.editingIndex === index) { this.cancelEdit(); }
            else if (this.editingIndex > index) { this.editingIndex--; }
        },
        // restoreSkill puts a removed skill back where it was in its routine
        // (skills), whether or not that routine is still on screen.
        restoreSkill(skills, index, skill, side) {
            index = Math.min(index, skills.length);
            if (this.skillsOf(side) !== skills) {
                skills.splice(index, 0, skill);
                this.persist();
                this.renderRoutines();
                return;
            }
            this.expandedOf(side).splice(index, 0, false);
            skills.splice(index, 0, skill);
            if (this.editingSide === side && this.editingIndex !== null && this.editingIndex >= index) { this.editingIndex++; }
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
        async clearRoutine() {
            if (this.routine.length === 0 || !await this.askSure(`Remove every skill from ${this.currentRoutine().name}?`, 'Remove them')) { return; }
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
                filter: 'button:not(.card-toggle), #skill-form-wrapper', preventOnFilter: false, // the card's buttons (and a skill form in it) stay usable; its head (a button too) still drags, and a tap on it still opens it
                delay: 200, delayOnTouchOnly: true,       // on touch, press and hold so the page still scrolls
                animation: 150,
                onEnd: (event) => {
                    const from = event.oldDraggableIndex, to = event.newDraggableIndex;
                    if (from === undefined || from === to) { return; }
                    this.moveSkill(from, to, side);
                },
            });
        },

        // askUser asks on the page, answering with the button chosen (its
        // value), or null on Cancel; where a name is typed (value), Save
        // answers with what was typed.
        askUser({ title, text = '', value = null, buttons = [] }) {
            return new Promise((resolve) => {
                this.ask = { open: true, title, text, value, buttons, resolve };
                this.$nextTick(() => document.querySelector('#ask-dialog input, #ask-dialog .button.is-primary')?.focus());
            });
        },
        // askName asks for a name, answering with it trimmed, or null.
        async askName(title, value) {
            const answer = await this.askUser({ title, value, buttons: [{ label: 'Save', value: 'save', primary: true }] });
            return answer === 'save' && this.ask.value.trim() ? this.ask.value.trim() : null;
        },
        // askSure asks before something hard to take back.
        async askSure(title, label = 'Delete') {
            return (await this.askUser({ title, buttons: [{ label, value: 'yes', danger: true }] })) === 'yes';
        },
        answerAsk(value) {
            const resolve = this.ask.resolve;
            this.ask.open = false;
            resolve?.(value);
        },

        // showToast says what just happened, for a few seconds; undo, given,
        // puts an Undo button on it, and a little longer to reach it.
        showToast(message, type = 'info', undo = null) {
            this.toast = { show: true, message, type, undo };
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(() => this.toast.show = false, undo ? 6000 : 3000);
        },
        undoToast() {
            const undo = this.toast.undo;
            this.toast = { ...this.toast, show: false, undo: null };
            undo?.();
        },
    }
}
