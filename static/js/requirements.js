// requirements.js: the requirement sets page (views.RequirementsPage). Lists
// the sets saved in this browser (SetStore), opens one in the server-rendered
// editor, and saves, duplicates, deletes, exports and imports sets.
function requirementsPage() {
    return {
        sets: [],
        builtins: {}, // built-in sets by id, for duplicating
        editing: null, // {id, title}: the set open in the editor (id null until first saved)
        importing: false,
        importText: '',
        toast: '',

        init() {
            this.sets = SetStore.load();
            try { this.builtins = JSON.parse(document.getElementById('builtin-sets').textContent); } catch (e) { this.builtins = {}; }
            document.body.addEventListener('htmx:responseError', (event) => {
                if (event.detail.target.closest('#editor-area')) {
                    this.flash(`Couldn't open that: ${event.detail.xhr.responseText.trim()}`);
                }
            });
        },

        // open shows a set in the editor; id is the saved set it came from, if any.
        open(set, title, id = null) {
            this.editing = { id, title };
            htmx.ajax('POST', '/requirements/editor', {
                source: '#editor-area', target: '#editor-area', swap: 'innerHTML',
                values: { set: JSON.stringify(set) },
            }).then(() => window.scrollTo({ top: 0, behavior: 'smooth' }));
        },
        newSet() { this.open({ format: 1, name: '', rules: [] }, 'New requirements'); },
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

        // saveEditing posts the editor once more (so a field still being typed in is
        // included) and saves the set exactly as the server read it.
        async saveEditing() {
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
        },
        closeEditor() {
            this.editing = null;
            document.getElementById('editor-area').replaceChildren();
        },
        remove(id) {
            const saved = this.sets.find((s) => s.id === id);
            if (!saved || !confirm(`Delete ${saved.set.name}? Routines checked against it will stop being checked.`)) { return; }
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

        rulesCount(set) { return set.rules.length === 1 ? '1 rule' : `${set.rules.length} rules`; },

        flash(message) {
            this.toast = message;
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(() => { this.toast = ''; }, 3000);
        },
    };
}
