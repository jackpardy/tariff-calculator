// sheet.js: the tariff sheet's display options (views.TariffSheetPage). Each
// checkbox shows or hides part of the sheet, on screen and in print, and the
// choice is remembered in this browser.
(function () {
    // checkbox id -> [class set on #tariff-sheet when unticked, localStorage key]
    const options = {
        'show-names': ['hide-names', 'sheetShowNames'],
        'show-fields': ['hide-fields', 'sheetShowFields'],
    };

    function remembered(key) {
        try { return localStorage.getItem(key); } catch (e) { return null; }
    }
    function remember(key, value) {
        try { localStorage.setItem(key, value); } catch (e) { /* storage unavailable: still works for this visit */ }
    }

    document.addEventListener('DOMContentLoaded', () => {
        const sheet = document.getElementById('tariff-sheet');
        for (const [id, [hiddenClass, key]] of Object.entries(options)) {
            const box = document.getElementById(id);
            if (!box || !sheet) { continue; }
            const saved = remembered(key);
            if (saved !== null) { box.checked = saved === 'true'; }
            const apply = () => sheet.classList.toggle(hiddenClass, !box.checked);
            box.addEventListener('change', () => { remember(key, String(box.checked)); apply(); });
            apply();
        }
    });
})();
