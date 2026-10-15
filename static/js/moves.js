// moves.js: the organiser's timetable (views/timetable.templ). Every
// gymnast's move dropdown fills its list of flights from the one copy on the
// page (#move-options) the first time it's opened or focused, so the page
// carries the list once rather than once per gymnast.
(function () {
    const options = document.getElementById('move-options');
    if (!options) { return; }
    const fill = (select) => {
        if (select.dataset.filled) { return; }
        select.dataset.filled = '1';
        // The flights go after "Choose a flight", before "Take out of the timetable".
        select.insertBefore(options.content.cloneNode(true), select.options[1] || null);
    };
    document.addEventListener('toggle', (e) => {
        if (e.target.matches && e.target.matches('details.comp-move') && e.target.open) {
            e.target.querySelectorAll('select[data-moves]').forEach(fill);
        }
    }, true);
    document.addEventListener('focusin', (e) => {
        if (e.target.matches && e.target.matches('select[data-moves]')) { fill(e.target); }
    });
    // Unplaced gymnasts' dropdowns aren't inside a <details>: fill those now.
    document.querySelectorAll('select[data-moves]').forEach((s) => { if (!s.closest('details')) { fill(s); } });
})();
