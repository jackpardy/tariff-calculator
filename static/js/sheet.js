// sheet.js: the tariff sheet's display options (views.TariffSheetPage). Each
// checkbox with data-hides toggles that class on #tariff-sheet when unticked,
// hiding part of the sheet on screen and in print. Choices are remembered in
// this browser.
(function () {
    const storageKey = (box) => 'sheetShow:' + box.dataset.hides;

    function remembered(key) {
        try { return localStorage.getItem(key); } catch (e) { return null; }
    }
    function remember(key, value) {
        try { localStorage.setItem(key, value); } catch (e) { /* storage unavailable: still works for this visit */ }
    }

    document.addEventListener('DOMContentLoaded', () => {
        const sheet = document.getElementById('tariff-sheet');
        const boxes = [...document.querySelectorAll('input[data-hides]')];
        const fieldBoxes = boxes.filter((box) => box.dataset.hides.startsWith('hide-field-'));
        if (!sheet) { return; }

        const apply = () => {
            for (const box of boxes) { sheet.classList.toggle(box.dataset.hides, !box.checked); }
            // With every field hidden, drop the details block (and its spacing) entirely.
            sheet.classList.toggle('hide-fields', fieldBoxes.length > 0 && fieldBoxes.every((box) => !box.checked));
        };
        for (const box of boxes) {
            const saved = remembered(storageKey(box));
            if (saved !== null) { box.checked = saved === 'true'; }
            box.addEventListener('change', () => { remember(storageKey(box), String(box.checked)); apply(); });
        }
        apply();
    });
})();
