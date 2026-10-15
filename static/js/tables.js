// tables.js: wide tables on the competition pages.
//
// A table that scrolls sideways (in a .table-container) shows a line at the
// edge where there's more to see, left or right, so it's plain there is.
//
// Tables marked data-table-view="<id>" (each level's entries, say, all with
// the same columns) can be set up by whoever is looking, with the Columns
// menu (data-table-menu="<id>"): which columns show, long text wrapped or
// clipped to one line, and how wide each column is, by dragging the edge of
// its heading. The choice is kept in this browser, for every table of that id.
// On a phone, where such tables stack as cards, only hidden columns count.
(function () {
    // --- Lines at the edges with more to scroll ---
    function edges(box) {
        const more = box.scrollWidth - box.clientWidth > 1;
        box.classList.toggle('has-more-left', more && box.scrollLeft > 1);
        box.classList.toggle('has-more-right', more && box.scrollLeft + box.clientWidth < box.scrollWidth - 1);
    }
    function watchEdges() {
        document.querySelectorAll('.table-container').forEach((box) => {
            if (!box.dataset.edges) {
                box.dataset.edges = '';
                box.addEventListener('scroll', () => edges(box), { passive: true });
            }
            edges(box);
        });
    }

    // --- Set up by the viewer ---
    const wide = window.matchMedia('(min-width: 769px)');
    const key = (id) => 'tableView:' + id;
    function load(id) {
        try { return { hidden: [], widths: {}, clip: false, ...JSON.parse(localStorage.getItem(key(id)) || '{}') }; } catch (e) { return { hidden: [], widths: {}, clip: false }; }
    }
    function save(id, view) {
        try { localStorage.setItem(key(id), JSON.stringify(view)); } catch (e) { /* this visit only */ }
    }
    // A column is known by its heading's words; one without (a tick box) can't be hidden.
    const label = (th) => th.textContent.trim();
    const heads = (table) => [...(table.tHead?.rows[0]?.cells || [])];

    // A column's width when first fixed: as wide as its content wants, up to
    // maxStart, so clipping or resizing starts from something sensible.
    const maxStart = 320;

    function apply(id) {
        const view = load(id);
        document.querySelectorAll(`table[data-table-view="${CSS.escape(id)}"]`).forEach((table) => {
            const cells = heads(table);
            const sized = wide.matches && (view.clip || Object.keys(view.widths).length > 0);
            // Back to the table as it comes, with the hidden columns hidden...
            table.classList.remove('is-sized-view', 'is-clipped-view');
            table.style.width = '';
            cells.forEach((th, i) => {
                const name = label(th);
                const hide = name !== '' && view.hidden.includes(name);
                for (const row of table.rows) {
                    if (row.cells[i]) { row.cells[i].hidden = hide; }
                }
                th.style.width = '';
                if (name !== '' && !th.querySelector('.comp-col-grip')) { grip(id, th); }
            });
            // ...then, sized or clipped, each column measured at its natural
            // width (not stretched across the page), and fixed.
            if (sized) {
                table.style.width = 'max-content';
                const natural = cells.map((th) => Math.ceil(th.getBoundingClientRect().width));
                let total = 0;
                cells.forEach((th, i) => {
                    if (th.hidden) { return; }
                    const w = view.widths[label(th)] || Math.min(natural[i] || 80, maxStart);
                    th.style.width = w + 'px';
                    total += w;
                });
                table.style.width = total + 'px';
                table.classList.add('is-sized-view');
            }
            table.classList.toggle('is-clipped-view', sized && view.clip);
        });
        watchEdges();
    }

    // grip is the handle on a heading's right edge that resizes its column.
    function grip(id, th) {
        const handle = document.createElement('span');
        handle.className = 'comp-col-grip';
        handle.setAttribute('aria-hidden', 'true');
        th.append(handle);
        handle.addEventListener('pointerdown', (event) => {
            if (!wide.matches) { return; }
            event.preventDefault();
            const view = load(id);
            // Fix every column's width first, so only this one changes.
            heads(th.closest('table')).forEach((h) => {
                if (label(h) !== '' && !view.widths[label(h)]) { view.widths[label(h)] = Math.ceil(h.getBoundingClientRect().width); }
            });
            const start = event.clientX;
            const from = view.widths[label(th)];
            handle.setPointerCapture(event.pointerId);
            const move = (e) => {
                view.widths[label(th)] = Math.max(48, from + e.clientX - start);
                save(id, view);
                apply(id);
            };
            const up = () => {
                handle.removeEventListener('pointermove', move);
                handle.removeEventListener('pointerup', up);
            };
            handle.addEventListener('pointermove', move);
            handle.addEventListener('pointerup', up);
        });
    }

    // menu fills in a Columns menu: a tick for each column that can be hidden.
    function menu(details) {
        const id = details.dataset.tableMenu;
        const table = document.querySelector(`table[data-table-view="${CSS.escape(id)}"]`);
        const list = details.querySelector('[data-table-columns]');
        if (!table || !list) { details.hidden = true; return; }
        const view = load(id);
        // Every column with a heading, but the first (whose row it is), can be hidden.
        const names = heads(table).map(label).filter((name) => name !== '');
        list.replaceChildren(...names.slice(1).map((name) => {
            const row = document.createElement('label');
            row.className = 'checkbox comp-table-menu-row';
            const box = document.createElement('input');
            box.type = 'checkbox';
            box.checked = !view.hidden.includes(name);
            box.addEventListener('change', () => {
                const v = load(id);
                v.hidden = box.checked ? v.hidden.filter((h) => h !== name) : [...v.hidden, name];
                save(id, v);
                apply(id);
            });
            row.append(box, ' ' + name);
            return row;
        }));
        const clip = details.querySelector('[data-table-clip]');
        clip.checked = view.clip;
        clip.addEventListener('change', () => {
            const v = load(id);
            v.clip = clip.checked;
            save(id, v);
            apply(id);
        });
        details.querySelector('[data-table-reset]').addEventListener('click', () => {
            try { localStorage.removeItem(key(id)); } catch (e) { /* nothing kept */ }
            menu(details);
            apply(id);
        });
    }

    function init() {
        const ids = new Set();
        document.querySelectorAll('table[data-table-view]').forEach((t) => ids.add(t.dataset.tableView));
        document.querySelectorAll('[data-table-menu]').forEach(menu);
        ids.forEach(apply);
        watchEdges();
    }
    // Levels open later: their tables' edges are measured then.
    document.addEventListener('toggle', (event) => { if (event.target.open) { watchEdges(); } }, true);
    window.addEventListener('resize', () => { document.querySelectorAll('table[data-table-view]').forEach((t) => apply(t.dataset.tableView)); });
    if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', init); } else { init(); }
})();
