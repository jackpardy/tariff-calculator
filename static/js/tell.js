// tell.js: the About list of Tell the organiser (views/my.templ). Typing in
// its search box shows only the entries and events whose words match
// (ignoring case and accents), a whole event with its entries; ticking a
// whole event greys out its entries, which it covers. Without it, every
// choice shows and the server drops entries a ticked event covers.
(() => {
    const plain = (text) => text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
    for (const form of document.querySelectorAll('form[data-tell]')) {
        if ('tellReady' in form.dataset) {
            continue;
        }
        form.dataset.tellReady = '';
        const search = form.querySelector('[data-tell-search]');
        if (search) {
            search.addEventListener('input', () => {
                const words = plain(search.value).split(/\s+/).filter(Boolean);
                const matches = (el) => {
                    const text = plain(el.dataset.tellText || '');
                    return words.every((w) => text.includes(w));
                };
                for (const group of form.querySelectorAll('[data-tell-group]')) {
                    const event = group.querySelector('.comp-tell-event');
                    const whole = event && matches(event);
                    let any = whole;
                    for (const entry of group.querySelectorAll('.comp-tell-entry')) {
                        const show = whole || matches(entry);
                        entry.hidden = !show;
                        any = any || show;
                    }
                    if (event) {
                        event.hidden = !any;
                    }
                    group.hidden = !any;
                }
            });
            // Enter in the search box doesn't send the message.
            search.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                }
            });
        }
        for (const event of form.querySelectorAll('[data-tell-event]')) {
            const entries = event.closest('[data-tell-group]').querySelectorAll('input[name="entry"]');
            const cover = () => {
                for (const entry of entries) {
                    entry.disabled = event.checked;
                    entry.closest('label').classList.toggle('has-text-grey-light', event.checked);
                }
            };
            event.addEventListener('change', cover);
            cover();
        }
    }
})();
