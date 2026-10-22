// requirements.js: the requirement sets page (views.RequirementsPage). Lists
// the sets and levels saved in this browser (SetStore, LevelStore), opens one
// in its server-rendered editor, and saves, duplicates and deletes them;
// sets can also be exported and imported. Set routines are built in the
// Routine Builder (or made here from a routine already built) and edited there.
function requirementsPage() {
    return {
        sets: [],
        levels: [],
        builtinLevels: {}, // built-in levels by reference, for duplicating
        confirmClose: false, // Close asked about changes not saved
        asking: '', // the row asked about deleting: 'set:<id>' or 'level:<id>'
        savingAnyway: null, // a save with problems asked about: {where ('editor' or 'writer'), text, resolve}
        requirementNames: {}, // built-in requirements' names by reference
        editing: null, // {id, title, kind}: the set or level ('set' or 'level') open in the editor (id null until first saved)
        levelWriting: null, // requirements written for one of the open level's exercises: {key, id, title}
        levelKeepToo: false, // and kept in Your requirements too
        importing: false,
        routines: [], // the routines saved in this browser, to make a set routine of
        makingSetRoutine: false,
        makerStart: null, // {name, skills} the set routine maker starts from (a copy)
        makerFor: null,   // the level exercise ("first", "second") a set routine is made for
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
            this.confirmClose = false;
            htmx.ajax('POST', '/requirements/editor', {
                source: '#editor-area', target: '#editor-area', swap: 'innerHTML',
                values: { set: JSON.stringify(set) },
            }).then(() => { this.markClean(); window.scrollTo({ top: 0, behavior: 'smooth' }); });
        },
        // editorJSON is what the open editor would save now.
        editorJSON() {
            const form = document.querySelector('#editor-area form');
            return form?.dataset.setJson || form?.dataset.levelJson || '';
        },
        // markClean notes the editor as opened or saved: Close then just closes.
        markClean() { if (this.editing) { this.editing.clean = this.editorJSON(); } },
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
        // saveLevelToClub saves a level as a club level: the rules of the
        // requirements its exercises are checked against (each exercise's
        // first choice), one after another, under the level's name.
        async saveLevelToClub(coach, saved) {
            const level = saved.level;
            const rules = [];
            for (const ex of [level.first, level.second].filter(Boolean)) {
                const ref = (ex.options || []).find((r) => r);
                const set = this.sets.find((s) => s.id === ref)?.set || await this.builtinSet(ref);
                if (set) { rules.push(...JSON.parse(JSON.stringify(set.rules || []))); }
            }
            if (rules.length === 0) { this.flash(`${level.name} has nothing chosen to save as a club level yet.`); return; }
            this.saveToClub(coach, { id: saved.id, set: { format: 1, name: level.name, rules } });
        },
        // keepOnPage keeps a saved set or level (kind 'set' or 'level') on a
        // person's page (link: one of personLinks), where it's kept from then
        // on: on any phone, and safe if this browser forgets it.
        async keepOnPage(link, kind, saved) {
            try {
                await KeptRequirements.save(link.url, kind, saved);
                // A level's own requirements go with it, so it works on any phone.
                if (kind === 'level') {
                    for (const set of this.levelSets(saved.level)) {
                        if (!set.kept) { await KeptRequirements.save(link.url, 'set', set); }
                    }
                }
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
            copy.name = `Copy of ${set.name}`.slice(0, 80);
            this.open(copy, copy.name);
        },
        // duplicateBuiltin opens a copy of built-in requirements in the
        // editor: the server copies it (the page doesn't carry them all).
        duplicateBuiltin(id) {
            const ref = `builtin:${id}`;
            this.editing = { id: null, title: `Copy of ${this.requirementName(ref)}`, kind: 'set' };
            this.confirmClose = false;
            htmx.ajax('POST', '/requirements/editor', { source: '#editor-area', target: '#editor-area', swap: 'innerHTML', values: { copy: ref } })
                .then(() => { this.markClean(); window.scrollTo({ top: 0, behavior: 'smooth' }); });
        },
        // builtinSet is built-in requirements' JSON (ref "builtin:<id>"), as
        // the server's copy of them has it, or undefined.
        async builtinSet(ref) {
            if (!String(ref).startsWith('builtin:')) { return undefined; }
            try {
                const response = await fetch('/requirements/editor', { method: 'POST', body: new URLSearchParams({ copy: ref }) });
                if (!response.ok) { return undefined; }
                const form = new DOMParser().parseFromString(await response.text(), 'text/html').getElementById('set-editor');
                return JSON.parse(form.dataset.setJson);
            } catch (e) {
                return undefined;
            }
        },

        // --- Levels ---
        // openLevel shows a level in the level editor, which offers the sets
        // saved here as its options; id is the saved level it came from, if any.
        // clean, given, is the level as last saved (redrawn with changes still
        // to save); otherwise it's as opened.
        openLevel(level, title, id = null, clean = undefined) {
            this.editing = { id, title, kind: 'level' };
            const custom = this.sets.map((s) => ({ id: s.id, name: s.set.name, set_routine: (s.set.rules || []).some((r) => r.type === 'sequence') }));
            this.confirmClose = false;
            htmx.ajax('POST', '/requirements/level-editor', {
                source: '#editor-area', target: '#editor-area', swap: 'innerHTML',
                values: { level: JSON.stringify(level), custom: JSON.stringify(custom) },
            }).then(() => {
                if (clean === undefined) { this.markClean(); } else if (this.editing) { this.editing.clean = clean; }
                window.scrollTo({ top: 0, behavior: 'smooth' });
            });
        },
        newLevel() { this.openLevel({ format: 1, name: '', first: { options: [''] }, second: { options: [''] } }, 'New level'); },
        editLevel(id) {
            const saved = this.levels.find((l) => l.id === id);
            if (saved) { this.openLevel(saved.level, `Edit ${saved.level.name}`, id); }
        },
        duplicateLevel(level) {
            const copy = JSON.parse(JSON.stringify(level));
            copy.name = `Copy of ${level.name}`.slice(0, 80);
            this.openLevel(copy, copy.name);
        },
        duplicateBuiltinLevel(ref) { if (this.builtinLevels[ref]) { this.duplicateLevel(this.builtinLevels[ref]); } },
        // levelSets are the user's own requirements a level uses.
        levelSets(level) {
            const refs = [...(level.first?.options || []), ...(level.second?.options || [])];
            return this.sets.filter((s) => refs.includes(s.id));
        },
        // levelDeleteText is what deleting a level loses, asked on its row.
        levelDeleteText(saved) {
            return `Delete ${saved.level.name}${saved.kept ? ', here and from your page' : ''}? Routines doing its exercises will stop being paired by it.`;
        },
        removeLevel(id) {
            const saved = this.levels.find((l) => l.id === id);
            if (!saved) { return; }
            KeptRequirements.drop('level', saved);
            this.levels = this.levels.filter((l) => l.id !== id);
            LevelStore.save(this.levels);
            // Requirements written for it alone go with it, unless another level uses them.
            const orphans = this.levelSets(saved.level).filter((s) => s.inLevel && !this.levels.some((l) => this.levelSets(l.level).includes(s)));
            for (const s of orphans) { KeptRequirements.drop('set', s); }
            this.sets = this.sets.filter((s) => !orphans.includes(s));
            SetStore.save(this.sets);
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
            const first = firstTry(form);
            const response = await fetch('/requirements/editor', { method: 'POST', body: saveBody(form) });
            if (!response.ok) { this.flash(`Couldn't save: ${(await response.text()).trim()}`); return; }
            const html = await response.text();
            const shown = swapIn(form, html);
            const fresh = new DOMParser().parseFromString(html, 'text/html').getElementById('set-editor');
            const set = JSON.parse(fresh.dataset.setJson);
            const described = JSON.parse(fresh.dataset.described);
            if (fresh.dataset.problems !== '0' && first) { this.flash('Fix the problems listed at the top first.'); shown.scrollIntoView({ block: 'start' }); return; }
            if (!set.name) { this.flash('Give the requirements a name first.'); return; }
            if (fresh.dataset.problems !== '0' && !await this.askSaveAnyway('editor', 'These have problems, listed at the top. Save them anyway?')) { return; }

            const existing = this.sets.find((s) => s.id === this.editing.id);
            if (existing) {
                Object.assign(existing, { set, described });
            } else {
                this.editing.id = SetStore.newId();
                this.sets.push({ id: this.editing.id, set, described });
            }
            SetStore.save(this.sets);
            this.editing.title = `Edit ${set.name}`;
            this.markClean();
            this.flash(`Saved ${set.name}.`);
            await this.keepAgain('set', this.sets.find((s) => s.id === this.editing.id));
        },
        async saveLevel() {
            const form = document.getElementById('level-editor');
            if (!form || !this.editing) { return; }
            const first = firstTry(form);
            const response = await fetch('/requirements/level-editor', { method: 'POST', body: saveBody(form) });
            if (!response.ok) { this.flash(`Couldn't save: ${(await response.text()).trim()}`); return; }
            const html = await response.text();
            const shown = swapIn(form, html);
            const fresh = new DOMParser().parseFromString(html, 'text/html').getElementById('level-editor');
            const level = JSON.parse(fresh.dataset.levelJson);
            if (fresh.dataset.problems !== '0' && first) { this.flash('Fix the problems listed at the top first.'); shown.scrollIntoView({ block: 'start' }); return; }
            if (!level.name) { this.flash('Give the level a name first.'); return; }
            if (fresh.dataset.problems !== '0' && !await this.askSaveAnyway('editor', 'This level has problems, listed at the top. Save it anyway?')) { return; }

            // A level's own requirements are its own: one another level uses
            // too (this is a copy of it) is copied for this one.
            let copied = false;
            for (const ex of [level.first, level.second].filter(Boolean)) {
                ex.options = ex.options.map((ref) => {
                    const own = this.sets.find((st) => st.id === ref && st.inLevel);
                    if (!own || !this.levels.some((l) => l.id !== this.editing.id && this.levelSets(l.level).includes(own))) { return ref; }
                    const copy = { ...JSON.parse(JSON.stringify(own)), id: SetStore.newId(), kept: undefined };
                    this.sets.push(copy);
                    copied = true;
                    return copy.id;
                });
            }
            if (copied) { SetStore.save(this.sets); }

            const existing = this.levels.find((l) => l.id === this.editing.id);
            if (existing) {
                existing.level = level;
            } else {
                this.editing.id = LevelStore.newId();
                this.levels.push({ id: this.editing.id, level });
            }
            LevelStore.save(this.levels);
            this.editing.title = `Edit ${level.name}`;
            // The editor shows the copies it now uses.
            if (copied) { this.openLevel(level, this.editing.title, this.editing.id); } else { this.markClean(); }
            this.flash(`Saved ${level.name}.`);
            await this.keepAgain('level', this.levels.find((l) => l.id === this.editing.id));
        },
        // closeEditor closes the editor; with changes not saved, it asks first
        // (confirmClose shows the question in the editor's head), unless
        // they're to be discarded (discard).
        closeEditor(discard = false) {
            if (!discard && this.editing && (this.editorJSON() !== this.editing.clean || this.levelWriting)) {
                this.confirmClose = true;
                return;
            }
            this.confirmClose = false;
            this.answerSaveAnyway(false);
            this.editing = null;
            this.cancelForLevel();
            document.getElementById('editor-area').replaceChildren();
        },

        // --- Requirements written for one of a level's exercises ---
        // writeForLevel opens the requirements editor under the level editor,
        // for its exercise key ("first" or "second"): on its own requirements
        // (ref, changed in place), a copy of a built-in one, or new ('').
        writeForLevel(key, ref, exercise) {
            const saved = this.sets.find((s) => s.id === ref);
            let values;
            if (saved) {
                this.levelWriting = { key, id: saved.id, title: `Change ${saved.set.name} (${exercise})` };
                this.levelKeepToo = !saved.inLevel;
                values = { set: JSON.stringify(saved.set) };
            } else if (ref.startsWith('builtin:')) {
                this.levelWriting = { key, id: null, title: `A copy of ${this.requirementName(ref)} (${exercise})` };
                this.levelKeepToo = false;
                values = { copy: ref };
            } else {
                const name = document.getElementById('level-name')?.value.trim();
                this.levelWriting = { key, id: null, title: `New requirements (${exercise})` };
                this.levelKeepToo = false;
                values = { set: JSON.stringify({ format: 1, name: name ? `${name} · ${exercise.split(':')[0].toLowerCase()}` : '', rules: [] }) };
            }
            htmx.ajax('POST', '/requirements/editor', { source: '#level-write-area', target: '#level-write-area', swap: 'innerHTML', values })
                .then(() => document.querySelector('.level-writer')?.scrollIntoView({ behavior: 'smooth', block: 'start' }));
        },
        // saveForLevel saves the requirements written for the level's
        // exercise, as the editor has them, and chooses them for it: kept in
        // Your requirements only if ticked (otherwise they're the level's).
        async saveForLevel() {
            const form = document.querySelector('#level-write-area #set-editor');
            const writing = this.levelWriting;
            if (!form || !writing) { return; }
            const first = firstTry(form);
            const response = await fetch('/requirements/editor', { method: 'POST', body: saveBody(form) });
            if (!response.ok) { this.flash(`Couldn't save: ${(await response.text()).trim()}`); return; }
            const html = await response.text();
            swapIn(form, html);
            const fresh = document.querySelector('#level-write-area #set-editor');
            const set = JSON.parse(fresh.dataset.setJson);
            if (fresh.dataset.problems !== '0' && first) { this.flash('Fix the problems listed at the top first.'); fresh.scrollIntoView({ block: 'start' }); return; }
            if (!set.name) { this.flash('Give the requirements a name first.'); fresh.querySelector('[name=name]')?.focus(); return; }
            if (fresh.dataset.problems !== '0' && !await this.askSaveAnyway('writer', 'These have problems, listed at the top. Save them anyway?')) { return; }
            const described = JSON.parse(fresh.dataset.described || '[]');
            let saved = this.sets.find((s) => s.id === writing.id);
            if (saved) {
                Object.assign(saved, { set, described });
            } else {
                saved = { id: SetStore.newId(), set, described };
                this.sets.push(saved);
            }
            if (this.levelKeepToo) { delete saved.inLevel; } else { saved.inLevel = true; }
            SetStore.save(this.sets);
            await this.keepAgain('set', saved);
            this.chooseForLevel(writing.key, saved);
            this.cancelForLevel();
            this.flash(`Saved ${set.name}, for this level.`);
        },
        // chooseForLevel puts saved requirements in the level editor's
        // exercise (key), and redraws it.
        chooseForLevel(key, saved) {
            const editor = document.getElementById('level-editor');
            if (!editor) { return; }
            const custom = editor.querySelector('[name=custom]');
            const list = JSON.parse(custom.value || '[]').filter((c) => c.id !== saved.id);
            list.push({ id: saved.id, name: saved.set.name, set_routine: false });
            custom.value = JSON.stringify(list);
            const select = editor.querySelector(`[name="${key}.0"]`);
            if (!select) { return; }
            let option = [...select.options].find((o) => o.value === saved.id);
            if (!option) { option = new Option(saved.set.name, saved.id); select.add(option); }
            select.value = saved.id;
            select.dispatchEvent(new Event('change', { bubbles: true }));
        },
        cancelForLevel() {
            this.levelWriting = null;
            document.getElementById('level-write-area')?.replaceChildren();
        },
        // setDeleteText is what deleting requirements or a set routine
        // loses, asked on its row: the levels using it say so, as they'll lose it.
        setDeleteText(saved) {
            const using = this.levels.filter((l) => this.levelSets(l.level).includes(saved)).map((l) => l.level.name);
            const levels = using.length === 0 ? '' : ` ${using.length === 1 ? 'The level' : 'The levels'} ${using.join(', ')} ${using.length === 1 ? 'uses' : 'use'} it, and will lose it.`;
            const what = this.isSetRoutine(saved.set) ? 'Routines started from it keep their skills.' : 'Routines checked against it will stop being checked.';
            return `Delete ${saved.set.name}${saved.kept ? ', here and from your page' : ''}? ${what}${levels}`;
        },
        remove(id) {
            const saved = this.sets.find((s) => s.id === id);
            if (!saved) { return; }
            KeptRequirements.drop('set', saved);
            this.sets = this.sets.filter((s) => s.id !== id);
            SetStore.save(this.sets);
            if (this.editing?.id === id) { this.closeEditor(); }
            this.flash(`Deleted ${saved.set.name}.`);
        },

        // exportSet downloads a set as a .json file to share.
        exportSet(set) { this.download(set, set.name || 'requirements'); },
        // exportLevel downloads a level with the requirements of yours it
        // uses ({level, sets: {id: set}}, as a competition's own levels are),
        // so it opens anywhere.
        exportLevel(level) {
            const sets = {};
            for (const s of this.levelSets(level)) { sets[s.id] = s.set; }
            this.download({ level, sets }, level.name || 'level');
        },
        download(data, name) {
            const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
            const link = Object.assign(document.createElement('a'), {
                href: URL.createObjectURL(blob),
                download: name.replace(/[^\w\- ]+/g, '').trim().replace(/\s+/g, '-').toLowerCase() + '.json',
            });
            link.click();
            URL.revokeObjectURL(link.href);
        },
        // importPasted opens what was pasted or chosen: requirements, or a
        // level (with the requirements it brings, saved here as its own).
        importPasted() {
            let data;
            try { data = JSON.parse(this.importText); } catch (e) { this.flash("That can't be read: it isn't requirements or a level."); return; }
            const level = data?.level?.first ? data.level : data?.first ? data : null;
            if (!level && !Array.isArray(data?.rules)) { this.flash("That isn't requirements or a level."); return; }
            this.importing = false;
            this.importText = '';
            if (!level) { this.open(data, 'Imported requirements'); return; }
            // The requirements it brings: new ones of this level's own here.
            const ids = {};
            for (const [ref, set] of Object.entries(data.sets || {})) {
                if (!Array.isArray(set?.rules)) { continue; }
                const saved = { id: SetStore.newId(), set, inLevel: true };
                this.sets.push(saved);
                ids[ref] = saved.id;
            }
            if (Object.keys(ids).length > 0) { SetStore.save(this.sets); }
            for (const ex of [level.first, level.second].filter(Boolean)) {
                ex.options = (ex.options || []).map((ref) => ids[ref] || ref);
            }
            this.openLevel(level, 'Imported level');
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
        // requirementSets are Your requirements: not set routines, nor those
        // written for a level only (inLevel), which its exercise lists.
        requirementSets() { return this.sets.filter((s) => !this.isSetRoutine(s.set) && !s.inLevel); },
        // elementsOf is a set routine's elements by name.
        elementsOf(set) { return (set.rules.find((r) => r.type === 'sequence')?.sequence || []).map((m) => m.label || 'Element'); },
        // addSetRoutine keeps a set routine built here (setRoutineMaker); made
        // for a level's exercise, it's chosen there.
        addSetRoutine(set) {
            const id = SetStore.newId();
            this.sets.push({ id, set });
            SetStore.save(this.sets);
            this.makingSetRoutine = false;
            this.flash(`Saved the set routine ${set.name}.`);
            const key = this.makerFor;
            this.makerFor = null;
            const form = document.getElementById('level-editor');
            if (key && form && this.editing?.kind === 'level') {
                const level = JSON.parse(form.dataset.levelJson);
                const ex = key === 'second' ? level.second : level.first;
                if (ex) {
                    const empty = ex.options.indexOf('');
                    if (empty >= 0) { ex.options[empty] = id; } else { ex.options.push(id); }
                    this.openLevel(level, this.editing.title, this.editing.id, this.editing.clean); // still to save
                }
            }
        },
        // makeSetRoutineFor opens the set routine maker for a level's exercise.
        makeSetRoutineFor(key) {
            this.makerFor = key;
            this.makerStart = null;
            this.makingSetRoutine = true;
            this.$nextTick(() => document.getElementById('set-routine-maker')?.scrollIntoView({ behavior: 'smooth', block: 'start' }));
        },
        // copyBuiltinSetRoutine opens the set routine maker on a copy of a
        // competition's set routine, to change and save as one's own.
        async copyBuiltinSetRoutine(ref, name) {
            let skills;
            try {
                const response = await fetch('/set-routine', { method: 'POST', body: new URLSearchParams({ requirementSet: ref }) });
                skills = response.ok ? (await response.json()).skills : undefined;
            } catch (e) { /* below */ }
            if (!skills) { this.flash(`Couldn't open ${name}.`); return; }
            this.makerFor = null;
            this.makerStart = { name: `Copy of ${name}`.slice(0, 60), skills };
            this.makingSetRoutine = false;
            this.$nextTick(() => {
                this.makingSetRoutine = true;
                this.$nextTick(() => document.getElementById('set-routine-maker')?.scrollIntoView({ behavior: 'smooth', block: 'start' }));
            });
        },
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

        // askSaveAnyway asks beside the Save just pressed (where: 'editor' or
        // 'writer') whether to save with problems, answering true or false.
        askSaveAnyway(where, text) {
            this.savingAnyway?.resolve(false);
            return new Promise((resolve) => { this.savingAnyway = { where, text, resolve }; });
        },
        answerSaveAnyway(yes) {
            const resolve = this.savingAnyway?.resolve;
            this.savingAnyway = null;
            resolve?.(yes);
        },
        flash(message) {
            this.toast = message;
            clearTimeout(this._toastTimer);
            this._toastTimer = setTimeout(() => { this.toast = ''; }, 3000);
        },
    };
}

// setRoutineMaker builds a set routine on the requirements page, skill by
// skill, with the Routine Builder's own search, builder and skill card (the
// compact /skill-form): the card's Add puts the skill on the list, and Save
// set routine saves it (the page adds it, on set-routine-made). It gives the
// form what the builder's component would: the search, the steppers, Add.
function setRoutineMaker() {
    return {
        name: '',
        skills: [],
        // startFrom fills it from a copy the page asked for (makerStart).
        startFrom() {
            if (!this.makerStart) { return; }
            this.name = this.makerStart.name;
            this.skills = this.makerStart.skills;
            this.makerStart = null;
        },
        busy: false,
        query: '', pickerTab: '', picked: null, pickedShape: null, // the form's picker
        routines: [], // no "Add to" choice: it's this list
        addTargets() { return []; },
        addTarget() { return true; }, // always this list
        addToValue() { return ''; },
        // previousSkill is the list's last skill, as JSON, for a body landing to take off where it landed.
        previousSkill() { return this.skills.length ? JSON.stringify(this.skills[this.skills.length - 1]) : ''; },
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
        async addFromForm() {
            if (this.busy) { return; }
            this.busy = true;
            try {
                const response = await fetch('/calculate-skill', { method: 'POST', body: new URLSearchParams(new FormData(document.getElementById('main-form'))) });
                if (!response.ok) { throw new Error((await response.text()).trim()); }
                this.skills.push(await response.json());
                const label = document.getElementById('custom-name');
                if (label) { label.value = ''; label.closest('details')?.removeAttribute('open'); }
                this.query = '';
            } catch (e) {
                this.$dispatch('flash', `Can't add that skill: ${e.message}`);
            } finally {
                this.busy = false;
            }
        },
        updateFromForm() {},
        cancelEdit() {},
        move(i, by) {
            const [s] = this.skills.splice(i, 1);
            this.skills.splice(i + by, 0, s);
        },
        async save() {
            const name = this.name.trim() || 'Set routine';
            this.busy = true;
            try {
                const response = await fetch('/requirements/set-routine', { method: 'POST', body: new URLSearchParams({ routineData: JSON.stringify(this.skills), name }) });
                if (!response.ok) { throw new Error((await response.text()).trim()); }
                this.$dispatch('set-routine-made', await response.json());
            } catch (e) {
                this.$dispatch('flash', `Couldn't save it: ${e.message}`);
            } finally {
                this.busy = false;
            }
        },
    };
}
