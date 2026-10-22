// fill.js: the organiser's timetable (views/timetable.templ). Long lists of
// choices (the flights a gymnast can move to, the people who can take a
// seat, the days and areas a flight can move to) are on the page once, in a
// <template>; each dropdown marked
// data-fill="<its id>" fills from it the first time it's opened or focused,
// after its first choice ("Choose a flight", "Choose a person"). A seat's
// people come in two groups: who can take it, and the rest.
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
        // A flight's "Move a gymnast" offers its running order, read from the
        // page rather than repeated in every flight.
        if (select.dataset.fill === 'order' && !select.dataset.filled) {
            select.dataset.filled = '1';
            for (const li of select.closest('.comp-flight')?.querySelectorAll('.comp-order li[data-entry]') || []) {
                select.append(new Option(`${li.value}. ${li.dataset.name}`, li.dataset.entry));
            }
            return;
        }
        if (select.dataset.fill === 'seat-options' && !select.dataset.filled) {
            const options = document.getElementById(select.dataset.fill);
            if (!options) { return; }
            select.dataset.filled = '1';
            select.append(options.content.cloneNode(true));
            sortSeat(select);
            return;
        }
        fill(select);
    };
    // A seat's people: first those who can take the seat chosen beside it
    // and are free then, then those who can but are busy, then the rest, to
    // choose anyway (the change then says what it breaks). Each seat says
    // what it needs (data-need: an event's code and a role's letter, or "r"
    // or "m"), the flights it runs across (data-span) and who has it
    // (data-holder); each person what they can take (data-can, web.seatCan);
    // and #seat-busy, by span, who isn't free then, as places in the list.
    let busyBySpan = null;
    let places = null;
    const sortSeat = (people) => {
        const seat = people.form?.querySelector('select[name="seat"]')?.selectedOptions[0];
        const need = seat?.dataset.need;
        if (!need) { return; }
        if (busyBySpan === null) {
            try { busyBySpan = JSON.parse(document.getElementById('seat-busy')?.textContent || '{}'); } catch (e) { busyBySpan = {}; }
            places = new Map([...(document.getElementById('seat-options')?.content.querySelectorAll('option') || [])].map((o, i) => [o.value, i]));
        }
        const busyList = seat.dataset.span ? busyBySpan[seat.dataset.span] : null;
        const busy = new Set(busyList || []);
        const isBusy = (o) => busyList !== null && busyList !== undefined && o.value !== seat.dataset.holder && busy.has(places.get(o.value));
        const [event, role] = need.includes(':') ? need.split(':') : ['', need];
        const can = (o) => (o.dataset.can || '').split(' ').some((t) => event === '' ? t === role : t.startsWith(event + ':') && t.slice(event.length + 1).includes(role));
        const chosen = people.value;
        const all = [...people.querySelectorAll('option[data-can]')];
        people.querySelectorAll('optgroup').forEach((g) => g.remove());
        all.forEach((o) => o.remove());
        const groups = busyList ? [
            ["Can take this seat, free then", 'yes', all.filter((o) => can(o) && !isBusy(o))],
            ["Can take this seat, but busy then", 'busy', all.filter((o) => can(o) && isBusy(o))],
            ["Can't take this seat", 'no', all.filter((o) => !can(o))],
        ] : [["Can take this seat", 'yes', all.filter(can)], ["Can't take this seat", 'no', all.filter((o) => !can(o))]];
        for (const [label, mark, list] of groups) {
            if (list.length === 0) { continue; }
            const group = document.createElement('optgroup');
            group.label = label;
            group.dataset.mark = mark; // a ✓, ◷ or ✗ in the search's list (static/js/selectsearch.js)
            group.append(...list);
            people.append(group);
        }
        if (chosen) { people.value = chosen; }
    };
    document.addEventListener('change', (e) => {
        if (e.target.matches && e.target.matches('select[name="seat"]')) {
            const people = e.target.form?.querySelector('select[data-fill="seat-options"]');
            if (people?.dataset.filled) { sortSeat(people); }
        }
    });
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
