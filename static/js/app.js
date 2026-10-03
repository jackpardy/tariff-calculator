// app.js: the calculator page's Alpine component (x-data="tariffCalculatorStore()").
// Loaded with defer before Alpine, so the function exists when Alpine starts.
// The page's state: the routine (saved in localStorage) and UI state local to
// the page. Everything shown is rendered by the server; this component asks for
// it whenever the routine or the form changes.
function tariffCalculatorStore() {
    return {
        routine: [],
        expanded: [], // per-card expanded state, parallel to routine
        editingIndex: null,
        showEvaluation: false,
        lastInsertPosition: 1,
        draggedIndex: null, dropIndex: null, isDragging: false,
        isTouchDevice: false,
        busy: false, // an add or update is in flight
        toast: { show: false, message: '', type: 'info' },

        init() {
            this.isTouchDevice = ('ontouchstart' in window) || navigator.maxTouchPoints > 0;
            try {
                this.routine = JSON.parse(localStorage.getItem('trampolineRoutine') || '[]');
            } catch (e) {
                console.error('Failed to parse saved routine:', e);
                localStorage.removeItem('trampolineRoutine');
            }
            this.lastInsertPosition = this.routine.length + 1;

            // Alpine passes the same (mutated) array as old and new, so track the length here.
            let length = this.routine.length;
            this.$watch('routine', (routine) => {
                if (routine.length !== length) {
                    length = routine.length;
                    this.fillPositionSelect('insert-position');
                    this.fillPositionSelect('evaluation-insert-position');
                }
                this.renderRoutine();
                localStorage.setItem('trampolineRoutine', JSON.stringify(routine));
            });

            document.body.addEventListener('htmx:afterSwap', (event) => {
                if (event.detail.target.id === 'evaluation-preview') {
                    this.showEvaluation = true;
                    this.fillPositionSelect('evaluation-insert-position');
                }
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
                values: { routineData: JSON.stringify(this.routine) }
            }).catch(error => console.error('Routine render request error:', error));
        },
        // loadForm shows a fresh form, or the form for the routine skill at editIndex.
        loadForm(editIndex = null) {
            const values = { sortBy: localStorage.getItem('commonSkillSortBy') || '' };
            if (editIndex !== null) {
                values.skill = JSON.stringify(this.routine[editIndex]);
                values.editIndex = editIndex;
            }
            return htmx.ajax('POST', '/skill-form', { source: '#skill-form-wrapper', target: '#skill-form-wrapper', swap: 'innerHTML', values })
                .then(() => this.fillPositionSelect('insert-position'));
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
            } else if (path === '/skill-evaluation') {
                this.showToast(`Can't evaluate: ${reason}`, 'error');
            } else {
                this.showToast(`Request failed: ${reason}`, 'error');
            }
        },
        rememberSort(sortBy) { localStorage.setItem('commonSkillSortBy', sortBy); },

        // --- Insert-position dropdowns (they depend on the routine's length) ---
        // Called after the routine changes or the dropdown is swapped in; deliberately not
        // deferred with $nextTick, whose callbacks Alpine can hold back while it
        // initialises swapped-in content.
        fillPositionSelect(id) {
            const select = document.getElementById(id);
            if (!select) { return; }
            const maxPosition = this.routine.length + 1;
            let selected = (this.lastInsertPosition || maxPosition) + 1;
            if (selected > maxPosition || selected < 1) { selected = maxPosition; }
            select.replaceChildren(...Array.from({ length: maxPosition }, (_, i) =>
                new Option(`Position ${i + 1}${i + 1 === maxPosition ? ' (End)' : ''}`, i + 1)));
            select.value = String(selected);
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
        async addFromForm() {
            if (this.busy) { return; }
            this.busy = true;
            try {
                const skill = await this.calculateFormSkill();
                this.addSkill(skill, document.getElementById('insert-position')?.value);
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
        addSkill(skill, position = null) {
            const target = parseInt(position);
            if (!isNaN(target) && target >= 1 && target <= this.routine.length) {
                this.expanded.splice(target - 1, 0, false);
                this.routine.splice(target - 1, 0, skill);
                this.lastInsertPosition = target;
                this.showToast(`Skill added at position ${target}.`, 'info');
            } else {
                if (this.routine.length >= 10) { this.showToast('Warning: Routines typically have 10 skills.', 'warning'); }
                this.expanded.push(false);
                this.routine.push(skill);
                this.lastInsertPosition = this.routine.length;
                this.showToast('Skill added to end.', 'info');
            }
            this.showEvaluation = false;
            const customName = document.getElementById('custom-name');
            if (customName) { customName.value = ''; } // A label belongs to one skill
            this.fillPositionSelect('insert-position');
        },
        addEvaluatedSkillToRoutine() {
            const json = document.querySelector('#evaluation-preview [data-skill-data]')?.dataset.skillData;
            if (!json) { this.showToast('No evaluated skill data found to add.', 'warning'); return; }
            this.addSkill(JSON.parse(json), document.getElementById('evaluation-insert-position')?.value);
            this.closeEvaluation();
        },
        closeEvaluation() {
            this.showEvaluation = false;
            document.getElementById('evaluation-preview').replaceChildren();
        },
        editSkill(index) {
            this.editingIndex = index;
            this.showEvaluation = false;
            this.loadForm(index)
                .then(() => document.getElementById('skill-form-wrapper')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
                .catch(() => { this.showToast('Failed to load edit form.', 'error'); this.editingIndex = null; });
        },
        cancelEdit() {
            this.editingIndex = null;
            this.lastInsertPosition = this.routine.length + 1;
            this.showEvaluation = false;
            this.loadForm().catch(() => this.showToast('Failed to load the form.', 'error'));
        },

        // --- Changing the routine ---
        removeSkill(index) {
            if (index < 0 || index >= this.routine.length) { return; }
            this.expanded.splice(index, 1);
            this.routine.splice(index, 1);
            this.lastInsertPosition = this.routine.length + 1;
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
            if (this.routine.length === 0 || !confirm('Are you sure?')) { return; }
            this.routine = [];
            this.expanded = [];
            this.showToast('Routine cleared.', 'info');
            this.cancelEdit();
        },

        // --- Drag and drop (desktop) ---
        handleDragStart(event, index) {
            this.draggedIndex = index; this.isDragging = true;
            event.dataTransfer.effectAllowed = 'move';
            event.dataTransfer.setData('text/plain', index);
        },
        handleDragOver(event, index) { event.preventDefault(); if (this.draggedIndex !== null) { this.dropIndex = index; } },
        handleDragLeave(event) { if (!event.currentTarget.contains(event.relatedTarget)) { this.dropIndex = null; } },
        // handleDrop drops the dragged skill at insertion point index (0 = before the first card).
        handleDrop(event, index) {
            event.preventDefault();
            const from = this.draggedIndex;
            if (from !== null && index !== from && index !== from + 1) {
                this.moveSkill(from, from < index ? index - 1 : index);
                this.lastInsertPosition = this.routine.length + 1;
            }
            this.handleDragEnd();
        },
        handleDragEnd() { this.$nextTick(() => { this.draggedIndex = null; this.dropIndex = null; this.isDragging = false; }); },

        showToast(message, type = 'info') {
            this.toast = { show: true, message, type };
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(() => this.toast.show = false, 3000);
        },
    }
}
