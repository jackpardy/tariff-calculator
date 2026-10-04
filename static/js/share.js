// share.js: share links for routines, requirement sets and levels. A link
// carries what it shares after "#share=", compressed: the fragment never
// reaches the server, and nothing is stored there. Opening a link offers to add
// what it carries. Used by the calculator (routines, with the custom sets and
// levels they're checked against, and paired by level) and the requirements
// page (sets and levels; a level brings the custom sets it uses). Needs
// routines.js and sets.js.
const Share = (() => {
    const prefix = '#share=';

    const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;
    // listing joins "a", "b and c", or "a, b and c", skipping empty parts.
    const listing = (parts) => {
        const items = parts.filter(Boolean);
        return items.length < 2 ? items.join('') : `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`;
    };

    // --- Packing ---
    // A routine's skills without what the server works out again (names, tariffs).
    const portableSkill = ({ name, tariff, ...skill }) => skill;

    function toBase64URL(bytes) {
        let binary = '';
        for (const b of bytes) { binary += String.fromCharCode(b); }
        return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    }
    function fromBase64URL(text) {
        const binary = atob(text.replace(/-/g, '+').replace(/_/g, '/'));
        return Uint8Array.from(binary, (c) => c.charCodeAt(0));
    }
    async function pipe(bytes, stream) {
        return new Uint8Array(await new Response(new Blob([bytes]).stream().pipeThrough(stream)).arrayBuffer());
    }

    // encode packs a share as "z<data>" (compressed) or "j<data>" (plain JSON,
    // for browsers that can't compress).
    async function encode(share) {
        const bytes = new TextEncoder().encode(JSON.stringify(share));
        if (window.CompressionStream) {
            try { return 'z' + toBase64URL(await pipe(bytes, new CompressionStream('deflate-raw'))); } catch (e) { /* plain below */ }
        }
        return 'j' + toBase64URL(bytes);
    }
    async function decode(text) {
        let bytes = fromBase64URL(text.slice(1));
        if (text[0] === 'z') { bytes = await pipe(bytes, new DecompressionStream('deflate-raw')); }
        const share = JSON.parse(new TextDecoder().decode(bytes));
        if (!share || share.v !== 1) { throw new Error('not a share link this app understands'); }
        const list = (v) => (Array.isArray(v) ? v : []);
        return { routines: list(share.routines), sets: list(share.sets), levels: list(share.levels) };
    }

    // pack is the share for the chosen routines, sets and levels: each custom
    // set a routine or level uses travels with it ("set:<n>" into sets), as does
    // a routine's custom level ("level:<n>" into levels). A level routine's
    // other tabs travel in its exercises.
    function pack(routineIds, setIds, levelIds = []) {
        const savedSets = SetStore.load(), savedLevels = LevelStore.load();
        const sets = [], setIndex = new Map();
        const addSet = (id) => {
            if (!setIndex.has(id)) {
                const saved = savedSets.find((s) => s.id === id);
                if (!saved) { return undefined; }
                setIndex.set(id, sets.length);
                sets.push(saved.set);
            }
            return `set:${setIndex.get(id)}`;
        };
        setIds.forEach(addSet);
        const levels = [], levelIndex = new Map();
        const addLevel = (id) => {
            if (!levelIndex.has(id)) {
                const saved = savedLevels.find((l) => l.id === id);
                if (!saved) { return undefined; }
                const portable = (exercise) => ({ ...exercise, options: (exercise.options || []).map((ref) => (ref.startsWith('builtin:') ? ref : addSet(ref) || ref)) });
                const level = { ...saved.level, first: portable(saved.level.first || {}) };
                if (saved.level.second) { level.second = portable(saved.level.second); }
                levelIndex.set(id, levels.length);
                levels.push(level);
            }
            return `level:${levelIndex.get(id)}`;
        };
        levelIds.forEach(addLevel);
        const routines = RoutineStore.load().routines.filter((r) => routineIds.includes(r.id)).map((r) => {
            const out = { name: r.name, skills: r.skills.map(portableSkill) };
            if (r.requirements?.startsWith('builtin:')) { out.requirements = r.requirements; }
            else if (r.requirements) { out.requirements = addSet(r.requirements); }
            if (r.checks) { out.checks = r.checks; }
            if (r.level) {
                const portableRef = (ref) => (!ref || ref.startsWith('builtin:') ? ref : addSet(ref) || ref);
                out.level = r.level.startsWith('builtin-level:') ? r.level : addLevel(r.level);
                out.exercise = r.exercise === 2 ? 2 : 1;
                out.exercises = (r.exercises || []).map((e) => {
                    const exercise = { option: portableRef(e?.option) };
                    const slots = Object.entries(e?.slots || {}).map(([ref, slot]) => [portableRef(ref), { ...slot, skills: (slot.skills || []).map(portableSkill) }]);
                    if (slots.length > 0) { exercise.slots = Object.fromEntries(slots); }
                    return exercise;
                });
            }
            return out;
        });
        return { v: 1, routines, sets, levels };
    }

    // --- The share dialog ---
    function el(tag, attrs = {}, ...children) {
        const node = document.createElement(tag);
        for (const [k, v] of Object.entries(attrs)) {
            if (k === 'class') { node.className = v; } else if (k.startsWith('on')) { node.addEventListener(k.slice(2), v); } else { node.setAttribute(k, v); }
        }
        node.append(...children);
        return node;
    }
    function modal(title, ...body) {
        const close = () => { root.remove(); document.removeEventListener('keydown', onKey); };
        const onKey = (e) => { if (e.key === 'Escape') { close(); } };
        const root = el('div', { class: 'modal is-active share-modal' },
            el('div', { class: 'modal-background', onclick: close }),
            el('div', { class: 'modal-card', role: 'dialog', 'aria-modal': 'true', 'aria-label': title },
                el('section', { class: 'modal-card-body' },
                    el('div', { class: 'share-head' }, el('p', { class: 'title is-5 mb-0' }, title), el('button', { class: 'delete', type: 'button', 'aria-label': 'Close', onclick: close })),
                    ...body)));
        document.addEventListener('keydown', onKey);
        document.body.append(root);
        return { root, close };
    }
    // choices is a list of tick boxes, one per {id, label, detail}.
    function choices(items, chosen, onChange) {
        return el('div', { class: 'share-choices' }, ...items.map((item) => el('label', { class: 'checkbox share-choice' },
            el('input', { type: 'checkbox', value: item.id, ...(chosen.includes(item.id) ? { checked: '' } : {}), onchange: onChange }),
            el('span', {}, el('span', { class: 'share-choice-name' }, item.label), item.detail ? el('span', { class: 'share-choice-detail' }, item.detail) : ''))));
    }
    const ticked = (box) => [...box.querySelectorAll('input:checked')].map((i) => i.value);

    // open shares routines ('routines', from the calculator) or sets ('sets',
    // from the requirements page), with the given ones ticked.
    function open(kind, chosen = []) {
        const routines = kind === 'routines';
        const savedSets = SetStore.load();
        const items = routines
            ? RoutineStore.load().routines.map((r) => ({ id: r.id, label: r.name, detail: count(r.skills.length, 'skill', 'skills') }))
            : [...LevelStore.load().map((l) => ({ id: l.id, label: l.level.name, detail: 'level' })),
                ...savedSets.map((s) => ({ id: s.id, label: s.set.name, detail: count(s.set.rules.length, 'rule', 'rules') }))];
        if (items.length === 0) { alert(routines ? 'There are no routines to share yet.' : 'Save some requirements first, then share them.'); return; }

        const link = el('input', { class: 'input is-small share-link', readonly: '', 'aria-label': 'Share link' });
        const status = el('p', { class: 'share-status' });
        const qr = el('div', { class: 'share-qr' });
        const shareButton = el('button', { class: 'button is-primary', type: 'button' }, 'Share…');
        const copyButton = el('button', { class: 'button', type: 'button' }, 'Copy link');
        const list = choices(items, chosen, () => update());
        let current = '', version = 0;

        async function update() {
            const ids = ticked(list), mine = ++version;
            const empty = ids.length === 0;
            shareButton.disabled = copyButton.disabled = empty;
            if (empty) { link.value = ''; qr.replaceChildren(); status.textContent = routines ? 'Tick the routines to share.' : 'Tick the requirements to share.'; return; }
            const isLevel = (id) => id.startsWith('level-');
            const share = routines ? pack(ids, []) : pack([], ids.filter((id) => !isLevel(id)), ids.filter(isLevel));
            const url = `${location.origin}${routines ? '/' : '/requirements'}${prefix}${await encode(share)}`;
            if (mine !== version) { return; }
            current = url;
            link.value = url;
            const travelling = routines ? share.sets.length + share.levels.length : share.sets.length > ids.filter((id) => !id.startsWith('level-')).length;
            status.textContent = !travelling ? '' : routines ? "The requirements they're checked against go with them." : 'The requirements the levels use go with them.';
            try {
                const response = await fetch('/qr', { method: 'POST', body: new URLSearchParams({ text: url }) });
                if (mine !== version) { return; }
                if (response.ok) { qr.innerHTML = await response.text(); } else { qr.replaceChildren(el('p', { class: 'share-status' }, (await response.text()).trim())); }
            } catch (e) { qr.replaceChildren(); }
        }

        shareButton.addEventListener('click', async () => {
            if (navigator.share) {
                try { await navigator.share({ title: 'Trampoline routines', url: current }); } catch (e) { /* cancelled */ }
            } else { copy(); }
        });
        copyButton.addEventListener('click', copy);
        async function copy() {
            try { await navigator.clipboard.writeText(current); copyButton.textContent = 'Copied'; } catch (e) { link.select(); copyButton.textContent = 'Press copy'; }
            setTimeout(() => { copyButton.textContent = 'Copy link'; }, 2000);
        }

        modal(routines ? 'Share routines' : 'Share requirements',
            el('p', { class: 'share-intro' }, routines ? 'Tick the routines to share. Anyone who opens the link can add them to their own routines.' : 'Tick the requirements to share. Anyone who opens the link can add them to their own.'),
            list,
            el('div', { class: 'buttons share-buttons' }, shareButton, copyButton),
            link, status, qr,
            el('p', { class: 'share-note' }, 'Or scan the code with another phone.'));
        update();
    }

    // --- Receiving a link ---
    // receive offers to add what a link carries; called on page load.
    async function receive() {
        if (!location.hash.startsWith(prefix)) { return; }
        const clearHash = () => history.replaceState(null, '', location.pathname + location.search);
        let share;
        try {
            share = await decode(location.hash.slice(prefix.length));
        } catch (e) {
            clearHash();
            alert("That share link couldn't be read. It may have been cut short; ask for it again.");
            return;
        }
        // Routines are added on the calculator page.
        if (share.routines.length > 0 && location.pathname !== '/') { location.replace('/' + location.hash); return; }

        const routineItems = share.routines.map((r, i) => ({ id: `r${i}`, label: r.name || `Routine ${i + 1}`, detail: count((r.skills || []).length, 'skill', 'skills') }));
        const setItems = share.sets.map((s, i) => ({ id: `s${i}`, label: s.name || `Requirements ${i + 1}`, detail: 'requirements' }));
        const levelItems = share.levels.map((l, i) => ({ id: `l${i}`, label: l.name || `Level ${i + 1}`, detail: 'level' }));
        const items = [...routineItems, ...levelItems, ...setItems];
        const list = choices(items, items.map((i) => i.id), () => {});
        const add = el('button', { class: 'button is-primary', type: 'button' }, 'Add');
        const cancel = el('button', { class: 'button', type: 'button' }, 'Cancel');
        const what = listing([routineItems.length && count(routineItems.length, 'routine', 'routines'), levelItems.length && count(levelItems.length, 'level', 'levels'), setItems.length && count(setItems.length, 'list of requirements', 'lists of requirements')]);
        const dialog = modal('Shared with you',
            el('p', { class: 'share-intro' }, `This link has ${what}. Tick what to add.`),
            list,
            el('div', { class: 'buttons share-buttons' }, add, cancel));
        cancel.addEventListener('click', () => { dialog.close(); clearHash(); });
        dialog.root.querySelector('.modal-background').addEventListener('click', clearHash);
        add.addEventListener('click', () => {
            const ids = ticked(list);
            const indexes = (kind) => ids.filter((id) => id[0] === kind).map((id) => +id.slice(1));
            const added = adopt(share, indexes('r'), indexes('s'), indexes('l'));
            clearHash();
            sessionStorage.setItem('shareAdded', added);
            location.reload();
        });
    }

    // uniqueName adds " (2)", " (3)"... to a name already taken.
    function uniqueName(name, taken) {
        if (!taken.has(name)) { return name; }
        for (let n = 2; ; n++) {
            if (!taken.has(`${name} (${n})`)) { return `${name} (${n})`; }
        }
    }

    // adopt saves the chosen routines (by index), sets and levels, with the sets
    // and levels the routines are checked against and the sets the levels use.
    // A set or level already saved, identically, is reused.
    function adopt(share, routineIndexes, setIndexes, levelIndexes = []) {
        const sets = SetStore.load();
        const setIds = new Map(); // share index -> saved id
        const saveSet = (i) => {
            if (setIds.has(i) || !share.sets[i]) { return setIds.get(i); }
            const json = JSON.stringify(share.sets[i]);
            let saved = sets.find((s) => JSON.stringify(s.set) === json);
            if (!saved) {
                const set = { ...share.sets[i], name: uniqueName(share.sets[i].name || 'Shared requirements', new Set(sets.map((s) => s.set.name))) };
                saved = { id: SetStore.newId(), set };
                sets.push(saved);
            }
            setIds.set(i, saved.id);
            return saved.id;
        };
        setIndexes.forEach(saveSet);

        const levels = LevelStore.load();
        const levelIds = new Map(); // share index -> saved id
        const saveLevel = (i) => {
            if (levelIds.has(i) || !share.levels[i]) { return levelIds.get(i); }
            const local = (exercise) => ({ ...exercise, options: (exercise?.options || []).map((ref) => (ref.startsWith('set:') ? saveSet(+ref.slice(4)) || ref : ref)) });
            const level = { ...share.levels[i], first: local(share.levels[i].first) };
            if (share.levels[i].second) { level.second = local(share.levels[i].second); }
            const json = JSON.stringify(level);
            let saved = levels.find((l) => JSON.stringify(l.level) === json);
            if (!saved) {
                level.name = uniqueName(level.name || 'Shared level', new Set(levels.map((l) => l.level.name)));
                saved = { id: LevelStore.newId(), level };
                levels.push(saved);
            }
            levelIds.set(i, saved.id);
            return saved.id;
        };
        levelIndexes.forEach(saveLevel);

        const state = RoutineStore.load();
        const names = new Set(state.routines.map((r) => r.name));
        let first = null;
        for (const i of routineIndexes) {
            const shared = share.routines[i];
            if (!shared || !Array.isArray(shared.skills)) { continue; }
            const routine = { id: RoutineStore.newId(), name: uniqueName((shared.name || 'Shared routine').slice(0, 60), names), skills: shared.skills };
            names.add(routine.name);
            if (shared.requirements?.startsWith('builtin:')) { routine.requirements = shared.requirements; }
            else if (shared.requirements?.startsWith('set:')) { routine.requirements = saveSet(+shared.requirements.slice(4)); }
            if (shared.checks) { routine.checks = shared.checks; }
            if (shared.level?.startsWith('builtin-level:')) { routine.level = shared.level; }
            else if (shared.level?.startsWith('level:')) { routine.level = saveLevel(+shared.level.slice(6)); }
            if (routine.level) {
                const localRef = (ref) => (ref?.startsWith('set:') ? saveSet(+ref.slice(4)) || ref : ref);
                routine.exercise = shared.exercise === 2 ? 2 : 1;
                routine.exercises = (Array.isArray(shared.exercises) ? shared.exercises : [{}, {}]).map((e) => {
                    const exercise = { option: localRef(e?.option) };
                    if (e?.slots) { exercise.slots = Object.fromEntries(Object.entries(e.slots).map(([ref, slot]) => [localRef(ref), slot])); }
                    return exercise;
                });
            }
            state.routines.push(routine);
            first ??= routine.id;
        }
        SetStore.save(sets);
        LevelStore.save(levels);
        if (first) { state.current = first; }
        RoutineStore.save(state);
        return listing([routineIndexes.length && count(routineIndexes.length, 'routine', 'routines'), levelIds.size && count(levelIds.size, 'level', 'levels'), setIds.size && count(setIds.size, 'list of requirements', 'lists of requirements')]);
    }

    // After adding from a link, the page reloads; say what was added.
    function announce() {
        const added = sessionStorage.getItem('shareAdded');
        if (!added) { return; }
        sessionStorage.removeItem('shareAdded');
        const note = el('div', { class: 'notification is-success is-fixed-bottom-right share-added', role: 'status' }, `Added ${added}.`);
        document.body.append(note);
        setTimeout(() => note.remove(), 4000);
    }

    document.addEventListener('DOMContentLoaded', () => { announce(); receive(); });
    return { open, encode, decode };
})();
