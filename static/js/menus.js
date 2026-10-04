// menus.js: pop-up menus built from <details data-menu> (the builder's More
// menu, the tariff sheet's "Show on sheet") close when you click or tap
// anywhere outside them, or press Escape.
(() => {
    const openMenus = () => document.querySelectorAll('details[data-menu][open]');
    document.addEventListener('click', (event) => {
        for (const menu of openMenus()) {
            if (!menu.contains(event.target)) { menu.removeAttribute('open'); }
        }
    });
    document.addEventListener('keydown', (event) => {
        if (event.key !== 'Escape') { return; }
        for (const menu of openMenus()) {
            menu.removeAttribute('open');
            menu.querySelector('summary')?.focus();
        }
    });
})();
