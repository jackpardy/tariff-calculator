// requirements.js: the requirement sets page (views.RequirementsPage). Lists
// the sets and levels saved in this browser (SetStore, LevelStore), opens one
// in its server-rendered editor, and saves, duplicates and deletes them;
// sets can also be exported and imported. Set routines are built in the
// Routine Builder (or made here from a routine already built) and edited there.
function requirementsPage() {
    return {
        sets: [],
        levels: [],
        builtins: {}, // built-in sets by id, for duplicating
        builtinLevels: {}, // built-in levels by reference, for duplicating
        requirementNames: {}, // built-in requirements' names by reference
        editing: null, // {id, title, kind}: the set or level ('set' or 'level') open in the editor (id null until first saved)
        importing: false,
        routines: [], // the routines saved in this browser, to make a set routine of
        makingSetRoutine: false,
        fromRoutine: '',
        setRoutineName: '',
        importText: '',
        toast: '',
        coachLinks: [], // club coach pages saved in this browser, to save requirements to as a club level (ADR 0011)
        personLinks: [], // personal pages saved in this browser, to keep requirements on (ADR 0013)

        init() {
            this.sets = SetStore.load();
            this.levels = LevelStore.load();
            this.routines = RoutineStore.load().routines;
            this.coachLinks = RoutineStore.coachLinks();
            this.personLinks = KeptRequirements.links();
            // Those kept on a person's page, read in; the page's copies win.
            KeptRequirements.sync().then((changed) => {
                if (changed) { this.sets = SetStore.load(); this.levels = LevelStore.load(); }
            });
            this.$watch('fromRoutine', (id) => {
                const routine = this.routines.find((r) => r.id === id);
                if (routine && !this.setRoutineName) { this.setRoutineName = routine.name; }
            });
            try { this.builtins = JSON.parse(document.getElementById('builtin-sets').textContent); } catch (e) { this.builtins = {}; }
            try {
                const data = JSON.parse(document.getElementById('level-data').textContent);
                this.builtinLevels = data.levels || {};
                this.requirementNames = data.names || {};
            } catch (e) { /* no built-in levels */ }
            document.body.addEventListener('htmx:responseError', (event) => {
                if (event.detail.target.closest('#editor-area')) {
                    this.flash(`Couldn't open that: ${event.detail.xhr.responseText.trim()}`);
                }
            });
        },

        // open shows a set in the editor; id is the saved set it came from, if any.
        open(set, title, id = null) {
            this.editing = { id, title, kind: 'set' };
            htmx.ajax('POST', '/requirements/editor', {
                source: '#editor-area', target: '#editor-area', swap: 'innerHTML',
                values: { set: JSON.stringify(set) },
            }).then(() => window.scrollTo({ top: 0, behavior: 'smooth' }));
        },
        newSet() { this.open({ format: 1, name: '', rules: [] }, 'New requirements'); },
        // saveToClub posts saved requirements to a coach's club as one of its
        // levels (coach: one of coachLinks, whose coach's role the server
        // finds), which then opens; saving the same requirements again
        // changes the level.
        saveToClub(coach, saved) {
            const form = Object.assign(document.createElement('form'), { method: 'post', action: '/clubs/levels/save' });
            for (const [name, value] of [['link', coach.url], ['set', JSON.stringify(saved.set)], ['id', saved.id]]) {
                form.append(Object.assign(document.createElement('input'), { type: 'hidden', name, value }));
            }
            document.body.append(form);
            form.submit();
        },
        // keepOnPage keeps a saved set or level (kind 'set' or 'level') on a
        // person's page (link: one of personLinks), where it's kept from then
        // on: on any phone, and safe if this browser forgets it.
        async keepOnPage(link, kind, saved) {
            try {
                await KeptRequirements.save(link.url, kind, saved);
            } catch (e) {
                this.flash(`Couldn't keep it on your page: ${e.message}`);
                return;
            }
            if (kind === 'set') { this.sets = SetStore.load(); } else { this.levels = LevelStore.load(); }
            this.flash(`Kept ${kind === 'set' ? saved.set.name : saved.level.name} on your page.`);
        },
        // keepAgain saves a kept set or level back to its page after a change
        // here, saying so if it couldn't (the browser's copy is saved anyway).
        async keepAgain(kind, saved) {
            if (!saved.kept) { return; }
            try {
                await KeptRequirements.save(saved.kept, kind, saved);
            } catch (e) {
                this.flash(`Saved here, but not on your page (${e.message}): try again when there's a signal.`);
            }
        },
        edit(id) {
            const saved = this.sets.find((s) => s.id === id);
            if (saved) { this.open(saved.set, `Edit ${saved.set.name}`, id); }
        },
        duplicate(set) {
            const copy = JSON.parse(JSON.stringify(set));
            copy.name = `${set.name} (copy)`;
            this.open(copy, 'New requirements (copy)');
        },
        duplicateBuiltin(id) { if (this.builtins[id]) { this.duplicate(this.builtins[id]); } },

        // --- Levels ---
        // openLevel shows a level in the level editor, which offers the sets
        // saved here as its options; id is the saved level it came from, if any.
        openLevel(level, title, id = null) {
            this.editing = { id, title, kind: 'level' };
            const custom = this.sets.map((s) => ({ id: s.id, name: s.set.name, set_routine: (s.set.rules || []).some((r) => r.type === 'sequence') }));
            htmx.ajax('POST', '/requirements/level-editor', {
                source: '#editor-area', target: '#editor-area', swap: 'innerHTML',
                values: { level: JSON.stringify(level), custom: JSON.stringify(custom) },
            }).then(() => window.scrollTo({ top: 0, behavior: 'smooth' }));
        },
        newLevel() { this.openLevel({ format: 1, name: '', first: { options: [''] }, second: { options: [''] } }, 'New level'); },
        editLevel(id) {
            const saved = this.levels.find((l) => l.id === id);
            if (saved) { this.openLevel(saved.level, `Edit ${saved.level.name}`, id); }
        },
        duplicateLevel(level) {
            const copy = JSON.parse(JSON.stringify(level));
            copy.name = `${level.name} (copy)`;
            this.openLevel(copy, 'New level (copy)');
        },
        duplicateBuiltinLevel(ref) { if (this.builtinLevels[ref]) { this.duplicateLevel(this.builtinLevels[ref]); } },
        removeLevel(id) {
            const saved = this.levels.find((l) => l.id === id);
            if (!saved || !confirm(`Delete ${saved.level.name}${saved.kept ? ', here and from your page' : ''}? Routines doing its exercises will stop being paired by it.`)) { return; }
            KeptRequirements.drop('level', saved);
            this.levels = this.levels.filter((l) => l.id !== id);
            LevelStore.save(this.levels);
            if (this.editing?.id === id) { this.closeEditor(); }
            this.flash(`Deleted ${saved.level.name}.`);
        },
        requirementName(ref) {
            if (ref?.startsWith('builtin:')) { return this.requirementNames[ref] || ref; }
            return this.sets.find((s) => s.id === ref)?.set.name || 'requirements no longer saved';
        },
        // exerciseLines say what a level's exercises are, for its listing.
        exerciseLines(level) {
            const names = (exercise) => (exercise?.options || []).map((ref) => this.requirementName(ref)).join(' or ');
            if (!level.second) { return [`Both exercises: ${names(level.first)}`]; }
            return [`First exercise: ${names(level.first)}`, `Second exercise: ${names(level.second)}`];
        },

        // saveEditing posts the editor once more (so a field still being typed in is
        // included) and saves the set or level exactly as the server read it.
        async saveEditing() {
            if (this.editing?.kind === 'level') { return this.saveLevel(); }
            const form = document.getElementById('set-editor');
            if (!form || !this.editing) { return; }
            const response = await fetch('/requirements/editor', { method: 'POST', body: new URLSearchParams(new FormData(form)) });
            if (!response.ok) { this.flash(`Couldn't save: ${(await response.text()).trim()}`); return; }
            const html = await response.text();
            form.outerHTML = html;
            const fresh = new DOMParser().parseFromString(html, 'text/html').getElementById('set-editor');
            const set = JSON.parse(fresh.dataset.setJson);
            const described = JSON.parse(fresh.dataset.described);
            if (!set.name) { this.flash('Give the requirements a name first.'); return; }
            if (fresh.dataset.problems !== '0' && !confirm('These requirements have problems (listed in the editor). Save them anyway?')) { return; }

            const existing = this.sets.find((s) => s.id === this.editing.id);
            if (existing) {
                Object.assign(existing, { set, described });
            } else {
                this.editing.id = SetStore.newId();
                this.sets.push({ id: this.editing.id, set, described });
            }
            SetStore.save(this.sets);
            this.editing.title = `Edit ${set.name}`;
            this.flash(`Saved ${set.name}.`);
            await this.keepAgain('set', this.sets.find((s) => s.id === this.editing.id));
        },
        async saveLevel() {
            const form = document.getElementById('level-editor');
            if (!form || !this.editing) { return; }
            const response = await fetch('/requirements/level-editor', { method: 'POST', body: new URLSearchParams(new FormData(form)) });
            if (!response.ok) { this.flash(`Couldn't save: ${(await response.text()).trim()}`); return; }
            const html = await response.text();
            form.outerHTML = html;
            const fresh = new DOMParser().parseFromString(html, 'text/html').getElementById('level-editor');
            const level = JSON.parse(fresh.dataset.levelJson);
            if (!level.name) { this.flash('Give the level a name first.'); return; }
            if (fresh.dataset.problems !== '0' && !confirm('This level has problems (listed in the editor). Save it anyway?')) { return; }

            const existing = this.levels.find((l) => l.id === this.editing.id);
            if (existing) {
                existing.level = level;
            } else {
                this.editing.id = LevelStore.newId();
                this.levels.push({ id: this.editing.id, level });
            }
            LevelStore.save(this.levels);
            this.editing.title = `Edit ${level.name}`;
            this.flash(`Saved ${level.name}.`);
            await this.keepAgain('level', this.levels.find((l) => l.id === this.editing.id));
        },
        closeEditor() {
            this.editing = null;
            document.getElementById('editor-area').replaceChildren();
        },
        remove(id) {
            const saved = this.sets.find((s) => s.id === id);
            if (!saved || !confirm(`Delete ${saved.set.name}${saved.kept ? ', here and from your page' : ''}? Routines checked against it will stop being checked.`)) { return; }
            KeptRequirements.drop('set', saved);
            this.sets = this.sets.filter((s) => s.id !== id);
            SetStore.save(this.sets);
            if (this.editing?.id === id) { this.closeEditor(); }
            this.flash(`Deleted ${saved.set.name}.`);
        },

        // exportSet downloads a set as a .json file to share.
        exportSet(set) {
            const blob = new Blob([JSON.stringify(set, null, 2)], { type: 'application/json' });
            const link = Object.assign(document.createElement('a'), {
                href: URL.createObjectURL(blob),
                download: (set.name || 'requirements').replace(/[^\w\- ]+/g, '').trim().replace(/\s+/g, '-').toLowerCase() + '.json',
            });
            link.click();
            URL.revokeObjectURL(link.href);
        },
        importPasted() {
            let set;
            try { set = JSON.parse(this.importText); } catch (e) { this.flash("Those aren't requirements (it isn't valid JSON)."); return; }
            this.importing = false;
            this.importText = '';
            this.open(set, 'Imported requirements');
        },
        importFile(event) {
            const file = event.target.files?.[0];
            if (!file) { return; }
            file.text().then((text) => { this.importText = text; this.importPasted(); });
            event.target.value = '';
        },

        // --- Set routines: built in the Routine Builder, kept with the requirements ---
        isSetRoutine(set) { return (set?.rules || []).some((r) => r.type === 'sequence'); },
        setRoutines() { return this.sets.filter((s) => this.isSetRoutine(s.set)); },
        requirementSets() { return this.sets.filter((s) => !this.isSetRoutine(s.set)); },
        // elementsOf is a set routine's elements by name.
        elementsOf(set) { return (set.rules.find((r) => r.type === 'sequence')?.sequence || []).map((m) => m.label || 'Element'); },
        // saveFromRoutine saves a routine already built as a set routine.
        async saveFromRoutine() {
            const routine = this.routines.find((r) => r.id === this.fromRoutine);
            if (!routine) { return; }
            const name = this.setRoutineName.trim() || routine.name;
            const response = await fetch('/requirements/set-routine', { method: 'POST', body: new URLSearchParams({ routineData: JSON.stringify(routine.skills), name }) });
            if (!response.ok) { this.flash(`Couldn't save it: ${(await response.text()).trim()}`); return; }
            const set = await response.json();
            const id = SetStore.newId();
            this.sets.push({ id, set });
            SetStore.save(this.sets);
            // The routine can update it later (More → Save as a set routine).
            const state = RoutineStore.load();
            const saved = state.routines.find((r) => r.id === routine.id);
            if (saved) { saved.setRoutine = id; RoutineStore.save(state); }
            this.makingSetRoutine = false;
            this.fromRoutine = '';
            this.setRoutineName = '';
            this.flash(`Saved the set routine ${name}.`);
        },

        rulesCount(set) { return set.rules.length === 1 ? '1 rule' : `${set.rules.length} rules`; },

        flash(message) {
            this.toast = message;
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(() => { this.toast = ''; }, 3000);
        },
    };
}
