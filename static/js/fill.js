// fill.js: the organiser's timetable (views/timetable.templ). Long lists of
// choices (the flights a gymnast can move to, the people who can take a
// seat) are on the page once, in a <template>; each dropdown marked
// data-fill="<its id>" fills from it the first time it's opened or focused,
// after its first choice ("Choose a flight", "Choose a person").
(function () {
    // A move offers only flights of its gymnasts' discipline (data-group),
    // and not the flight they're in (data-except).
    const fill = (select) => {
        const options = document.getElementById(select.dataset.fill);
        if (!options || select.dataset.filled) { return; }
        select.dataset.filled = '1';
        const list = options.content.cloneNode(true);
        for (const o of [...list.querySelectorAll('option')]) {
            if (('group' in select.dataset && 'group' in o.dataset && o.dataset.group !== select.dataset.group) || o.value === select.dataset.except) { o.remove(); }
        }
        select.insertBefore(list, select.options[1] || null);
    };
    // A move puts its flights before "Take out of the timetable"; a seat puts
    // its people after "No one".
    const fillSelect = (select) => {
        if (select.dataset.fill === 'seat-options' && !select.dataset.filled) {
            const options = document.getElementById(select.dataset.fill);
            if (!options) { return; }
            select.dataset.filled = '1';
            select.append(options.content.cloneNode(true));
            return;
        }
        fill(select);
    };
    document.addEventListener('toggle', (e) => {
        if (e.target.tagName === 'DETAILS' && e.target.open) {
            e.target.querySelectorAll('select[data-fill]').forEach(fillSelect);
        }
    }, true);
    document.addEventListener('focusin', (e) => {
        if (e.target.matches && e.target.matches('select[data-fill]')) { fillSelect(e.target); }
        // A list's search box (static/js/selectsearch.js) searches the filled list.
        const box = e.target.closest?.('.select-search');
        const select = box?.nextElementSibling?.querySelector?.('select[data-fill]') || (box?.nextElementSibling?.matches?.('select[data-fill]') ? box.nextElementSibling : null);
        if (select) { fillSelect(select); }
    });
    // Dropdowns not inside a <details> (unplaced gymnasts) fill now.
    document.querySelectorAll('select[data-fill]').forEach((s) => { if (!s.closest('details')) { fillSelect(s); } });
})();
