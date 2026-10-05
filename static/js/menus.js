// menus.js: pop-up menus built from <details data-menu> (the builder's More
// menu, the tariff sheet's "Show on sheet") close when you click or tap
// anywhere outside them, or press Escape. An open menu is kept on screen: its
// panel is anchored to one side of its button, which can push it off the edge
// when the button wraps to the other side of a narrow screen.
(() => {
    const gutter = 8;
    const openMenus = () => document.querySelectorAll('details[data-menu][open]');
    const fit = (menu) => {
        const panel = menu.querySelector(':scope > :not(summary)');
        if (!panel) { return; }
        panel.style.transform = '';
        panel.style.boxSizing = 'border-box';
        panel.style.maxWidth = `calc(100vw - ${2 * gutter}px)`;
        const box = panel.getBoundingClientRect();
        const width = document.documentElement.clientWidth;
        let shift = 0;
        if (box.left < gutter) { shift = gutter - box.left; }
        else if (box.right > width - gutter) { shift = Math.max(gutter - box.left, width - gutter - box.right); }
        if (shift) { panel.style.transform = `translateX(${shift}px)`; }
    };
    document.addEventListener('toggle', (event) => {
        if (event.target.matches?.('details[data-menu]') && event.target.open) { fit(event.target); }
    }, true);
    window.addEventListener('resize', () => openMenus().forEach(fit));
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
