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
        expanded: [], // per-card expanded state, parallel to routine
        compareId: null, // the routine shown beside the current one, if any
        routineB: [],    // its skills (the same array as in routines)
        expandedB: [],   // per-card expanded state, parallel to routineB
        addTo: 'a',      // which column the skill adder adds to while comparing
        editingIndex: null,
        editingSide: 'a', // the column of the skill being edited
        busy: false, // an add or update is in flight
        pickerTab: 'jumps', // the skill picker's open category
        picked: null,       // the common skill last chosen in the picker
        query: '',          // the picker's search text; results replace the tabs while it's set
        toast: { show: false, message: '', type: 'info' },

        init() {
            const state = RoutineStore.load();
            this.routines = state.routines;
            this.currentId = state.current;
            this.routine = this.currentRoutine().skills;
            this.persist(); // settles a routine carried over from an older version
            this.customSets = SetStore.load();
            // Sets edited in another tab (the requirements page) are picked up here.
            window.addEventListener('storage', (event) => {
                if (event.key === 'trampolineRequirementSets') {
                    this.customSets = SetStore.load();
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
            const values = { side, routineData: JSON.stringify(this.skillsOf(side)), requirementSet: SetStore.payload(this.routineFor(side)?.requirements) };
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
        skillsOf(side) { return side === 'b' ? this.routineB : this.routine; },
        expandedOf(side) { return side === 'b' ? this.expandedB : this.expanded; },
        persist() { RoutineStore.save({ current: this.currentId, routines: this.routines }); },
        // setRequirements chooses the requirement set a column's routine is checked against.
        setRequirements(ref, side = 'a') {
            this.routineFor(side).requirements = ref || undefined;
            this.persist();
            this.renderRoutine(side);
        },
        // switchRoutine makes the routine with id current, leaving any edit. Choosing
        // the routine shown beside it swaps the columns.
        switchRoutine(id) {
            if (!this.routines.some((r) => r.id === id)) { return; }
            if (id === this.compareId) { this.swapColumns(); return; }
            if (this.editingIndex !== null) { this.cancelEdit(); }
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
            const copy = { id: RoutineStore.newId(), name: `${source.name} (copy)`, skills: JSON.parse(JSON.stringify(source.skills)) };
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
            this.addTo = 'a';
            this.renderRoutine('a');
        },
        // swapColumns makes the routine beside the current one current, and vice versa.
        swapColumns() {
            if (!this.compareId) { return; }
            [this.currentId, this.compareId] = [this.compareId, this.currentId];
            [this.expanded, this.expandedB] = [this.expandedB, this.expanded];
            const flip = (side) => (side === 'a' ? 'b' : 'a');
            this.editingSide = flip(this.editingSide);
            this.addTo = flip(this.addTo);
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
        // addFromForm adds the skill shown on the card to the end of the routine (while
        // comparing, the one chosen under "Add to").
        async addFromForm() {
            if (this.busy) { return; }
            this.busy = true;
            const side = this.compareId ? this.addTo : 'a';
            try {
                const skill = await this.calculateFormSkill();
                this.expandedOf(side).push(false);
                this.skillsOf(side).push(skill);
                const where = this.compareId ? ` to ${this.routineFor(side).name}` : '';
                this.showToast(`Added ${skill.custom_name || skill.name} (${skill.tariff.toFixed(1)})${where}.`, 'info');
                if (this.skillsOf(side).length > 10) { this.showToast('Note: an exercise has 10 skills.', 'warning'); }
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
