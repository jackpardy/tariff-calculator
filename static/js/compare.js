// compare.js: the compare page (views.ComparePage). Fills the two routine
// choices from the routines saved in this browser (A: the current routine, B:
// the next one) and loads the comparison whenever either changes.
(function () {
    document.addEventListener('DOMContentLoaded', () => {
        const state = RoutineStore.load();
        const selects = [document.getElementById('compare-a'), document.getElementById('compare-b')];
        const view = document.getElementById('comparison');

        for (const select of selects) {
            for (const r of state.routines) {
                select.append(new Option(`${r.name} (${r.skills.length})`, r.id));
            }
        }
        const currentIndex = state.routines.findIndex((r) => r.id === state.current);
        selects[0].value = state.routines[currentIndex].id;
        selects[1].value = state.routines[(currentIndex + 1) % state.routines.length].id;

        const compare = () => {
            const [a, b] = selects.map((s) => state.routines.find((r) => r.id === s.value));
            htmx.ajax('POST', '/compare', {
                source: view, target: view, swap: 'innerHTML',
                values: { aName: a.name, aData: JSON.stringify(a.skills), bName: b.name, bData: JSON.stringify(b.skills) },
            });
        };
        selects.forEach((s) => s.addEventListener('change', compare));
        view.addEventListener('htmx:responseError', (event) => {
            view.textContent = 'Could not compare: ' + event.detail.xhr.responseText;
        });

        if (state.routines.length < 2) {
            view.textContent = 'Save at least two routines to compare them (New or Duplicate in the Routine Builder).';
            return;
        }
        compare();
    });
})();
