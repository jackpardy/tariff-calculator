// selectsearch.js: a long list of skills you can search (the requirements
// editor's picks). A <select data-search-select="<placeholder>"> gets a
// search box in front of it, showing what's chosen; typing lists the options
// whose words match (ignoring case and accents), with their group, and
// choosing one sets the select and sends its change, as choosing from the list
// does (htmx posts the editor). Enter takes the first match, Escape closes.
// The select stays, beside it, for whoever would rather scroll. Run on load
// and on whatever htmx swaps in.
(function () {
    const plain = (text) => text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
    const most = 12;

    function enhance(select) {
        if (select.dataset.searchReady) { return; }
        select.dataset.searchReady = '';
        const box = document.createElement('div');
        box.className = 'select-search';
        const input = Object.assign(document.createElement('input'), {
            type: 'search', className: 'input is-small select-search-input', autocomplete: 'off',
            placeholder: select.dataset.searchSelect || 'Search',
        });
        input.setAttribute('aria-label', input.placeholder);
        const list = document.createElement('div');
        list.className = 'select-search-list';
        list.hidden = true;
        box.append(input, list);
        (select.closest('.select') || select).before(box);

        const options = () => [...select.options].filter((o) => o.value !== '').map((o) => ({
            o, text: plain(o.textContent + ' ' + (o.parentElement.tagName === 'OPTGROUP' ? o.parentElement.label : '')),
        }));
        const choose = (option) => {
            select.value = option.value;
            input.value = '';
            list.hidden = true;
            select.dispatchEvent(new Event('change', { bubbles: true }));
        };
        const show = () => {
            const words = plain(input.value).split(/\s+/).filter(Boolean);
            if (words.length === 0) { list.hidden = true; return; }
            const found = options().filter((x) => words.every((w) => x.text.includes(w))).slice(0, most);
            list.replaceChildren(...(found.length ? found.map(({ o }) => {
                const b = Object.assign(document.createElement('button'), { type: 'button', className: 'select-search-option' });
                b.append(o.textContent);
                if (o.parentElement.tagName === 'OPTGROUP') {
                    const g = Object.assign(document.createElement('span'), { className: 'select-search-group' });
                    g.textContent = o.parentElement.label;
                    b.append(g);
                }
                // Before the input loses focus, so the click lands.
                b.addEventListener('mousedown', (e) => { e.preventDefault(); choose(o); });
                return b;
            }) : [Object.assign(document.createElement('p'), { className: 'select-search-none', textContent: 'Nothing matches that.' })]));
            list.hidden = false;
        };
        input.addEventListener('input', show);
        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                const first = options().find((x) => plain(input.value).split(/\s+/).filter(Boolean).every((w) => x.text.includes(w)));
                if (first && input.value.trim() !== '') { choose(first.o); }
            } else if (e.key === 'Escape') {
                input.value = '';
                list.hidden = true;
            }
        });
        // A search box's own change mustn't post the form it's in.
        input.addEventListener('change', (e) => e.stopPropagation());
        input.addEventListener('blur', () => { list.hidden = true; });
    }

    const run = (root) => (root.querySelectorAll ? root : document).querySelectorAll('select[data-search-select]').forEach(enhance);
    if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', () => run(document)); } else { run(document); }
    document.addEventListener('htmx:load', (event) => run(event.detail.elt.parentElement || document));
})();
