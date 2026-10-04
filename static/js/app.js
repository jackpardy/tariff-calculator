// app.js: the calculator page's Alpine component (x-data="tariffCalculatorStore()").
// Loaded with defer before Alpine, so the function exists when Alpine starts.
// The page's state: the routines saved in this browser (RoutineStore, from
// routines.js), the current one's skills, and UI state local to the page.
// Everything shown is rendered by the server; this component asks for it
// whenever the routine or the form changes.
function tariffCalculatorStore() {
    return {
        routines: [],   // all saved routines: {id, name, skills}
        currentId: null,
        routine: [],    // the current routine's skills (the same array as in routines)
        customSets: [], // requirement sets saved in this browser (SetStore)
        expanded: [], // per-card expanded state, parallel to routine
        editingIndex: null,
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
                    this.renderRoutine();
                }
            });
            // One flag per card from the start, so moves can splice it in step with the routine.
            this.expanded = this.routine.map(() => false);

            this.$watch('routine', (routine) => {
                this.currentRoutine().skills = routine; // the routine may have been replaced (e.g. cleared)
                this.renderRoutine();
                this.persist();
            });

            document.body.addEventListener('htmx:afterSwap', (event) => {
                if (event.detail.target.id === 'routine-view') { this.makeSortable(); }
            });
            document.body.addEventListener('htmx:afterRequest', (event) => {
                if (event.detail.failed) { this.requestFailed(event.detail.requestConfig?.path, event.detail.xhr); }
            });

            this.renderRoutine();
            this.loadForm();
        },

        // --- Server-rendered views ---
        renderRoutine() {
            // Each view is its own request source, so its requests don't queue behind
            // the other's; a newer render replaces one still in flight (hx-sync).
            htmx.ajax('POST', '/routine', {
                source: '#routine-view', target: '#routine-view', swap: 'innerHTML',
                values: { routineData: JSON.stringify(this.routine), requirementSet: SetStore.payload(this.currentRoutine()?.requirements) }
            }).catch(error => console.error('Routine render request error:', error));
        },
        // loadForm shows a fresh "Add a skill" panel, or one loaded with the routine skill at editIndex.
        loadForm(editIndex = null) {
            const values = {};
            if (editIndex !== null) {
                values.skill = JSON.stringify(this.routine[editIndex]);
                values.editIndex = editIndex;
            }
            return htmx.ajax('POST', '/skill-form', { source: '#skill-form-wrapper', target: '#skill-form-wrapper', swap: 'innerHTML', values });
        },
        requestFailed(path, xhr) {
            const reason = xhr.status === 400 ? xhr.responseText.trim() : 'the server could not be reached';
            if (path === '/routine') {
                this.showToast(`Routine not updated: ${reason}`, 'error');
                // If nothing has rendered yet (e.g. a saved routine that no longer validates), say so in place.
                const view = document.getElementById('routine-view');
                if (view && !view.querySelector('#routine-skills')) {
                    const p = document.createElement('p');
                    p.className = 'has-text-danger';
                    p.textContent = `Couldn't show the saved routine (${reason}). Use Clear Routine to start again.`;
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
        persist() { RoutineStore.save({ current: this.currentId, routines: this.routines }); },
        // setRequirements chooses the requirement set the current routine is checked against.
        setRequirements(ref) {
            this.currentRoutine().requirements = ref || undefined;
            this.persist();
            this.renderRoutine();
        },
        // switchRoutine makes the routine with id current, leaving any edit.
        switchRoutine(id) {
            if (!this.routines.some((r) => r.id === id)) { return; }
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
        deleteRoutine() {
            const routine = this.currentRoutine(), name = routine.name;
            if (!confirm(`Delete ${name}? This can't be undone.`)) { return; }
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
        toggleExpanded(index) { this.expanded[index] = !this.expanded[index]; },
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
        // addFromForm adds the skill shown on the card to the end of the routine.
        async addFromForm() {
            if (this.busy) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                this.expanded.push(false);
                this.routine.push(skill);
                this.showToast(`Added ${skill.custom_name || skill.name} (${skill.tariff.toFixed(1)}).`, 'info');
                if (this.routine.length > 10) { this.showToast('Note: an exercise has 10 skills.', 'warning'); }
                this.clearLabel(); // a label belongs to one skill
                this.query = '';   // back to the picker for the next skill
            } catch (error) {
                this.showToast(`Can't add skill: ${error.message}`, 'error');
            } finally {
                this.busy = false;
            }
        },
        async updateFromForm() {
            const index = this.editingIndex;
            if (this.busy || index === null || index >= this.routine.length) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                this.routine.splice(index, 1, skill);
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
        editSkill(index) {
            this.editingIndex = index;
            this.loadForm(index)
                .then(() => document.getElementById('skill-form-wrapper')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
                .catch(() => { this.showToast('Failed to load edit form.', 'error'); this.editingIndex = null; });
        },
        cancelEdit() {
            this.editingIndex = null;
            this.loadForm().catch(() => this.showToast('Failed to load the form.', 'error'));
        },

        // --- Changing the routine ---
        removeSkill(index) {
            if (index < 0 || index >= this.routine.length) { return; }
            this.expanded.splice(index, 1);
            this.routine.splice(index, 1);
            if (this.editingIndex === index) { this.cancelEdit(); }
            else if (this.editingIndex > index) { this.editingIndex--; }
        },
        moveSkillUp(index) { if (index > 0) { this.moveSkill(index, index - 1); } },
        moveSkillDown(index) { if (index < this.routine.length - 1) { this.moveSkill(index, index + 1); } },
        // moveSkill moves the skill at from to index to, keeping its expanded and editing state with it.
        moveSkill(from, to) {
            this.expanded.splice(to, 0, this.expanded.splice(from, 1)[0] ?? false);
            this.routine.splice(to, 0, this.routine.splice(from, 1)[0]);
            if (this.editingIndex === from) { this.editingIndex = to; }
            else if (this.editingIndex !== null && from < this.editingIndex && to >= this.editingIndex) { this.editingIndex--; }
            else if (this.editingIndex !== null && from > this.editingIndex && to <= this.editingIndex) { this.editingIndex++; }
        },
        clearRoutine() {
            if (this.routine.length === 0 || !confirm(`Remove every skill from ${this.currentRoutine().name}?`)) { return; }
            this.routine = [];
            this.expanded = [];
            this.showToast('Routine cleared.', 'info');
            this.cancelEdit();
        },

        // --- Drag to reorder ---
        // makeSortable lets the cards be dragged into a new order (SortableJS). The view
        // is re-rendered after every change, so this runs for each new #routine-skills.
        makeSortable() {
            const list = document.getElementById('routine-skills');
            if (!list || !window.Sortable) { return; }
            Sortable.create(list, {
                draggable: '.routine-skill-container',
                filter: 'button', preventOnFilter: false, // the card's buttons stay clickable
                delay: 200, delayOnTouchOnly: true,       // on touch, press and hold so the page still scrolls
                animation: 150,
                onEnd: (event) => {
                    const from = event.oldDraggableIndex, to = event.newDraggableIndex;
                    if (from === undefined || from === to) { return; }
                    this.moveSkill(from, to);
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
