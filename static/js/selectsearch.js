// selectsearch.js: a long list of skills you can search (the requirements
// editor's picks). A <select data-search-select="<placeholder>"> gets a
// search box in front of it, showing what's chosen; typing lists the options
// whose words match (ignoring case and accents), with their group, and
// choosing one sets the select and sends its change, as choosing from the list
// does (htmx posts the editor). Enter takes the first match, Escape closes.
// A select with data-search-url asks it too (?q=, answering option values as
// JSON, best first), for matches its labels don't show: the skill lists ask
// the skill search, so another name for a skill or its FIG notation finds it.
// The select stays, beside it, for whoever would rather scroll. Run on load
// and on whatever htmx swaps in.
(function () {
    const plain = (text) => text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
    const most = 12;

    function enhance(select) {
        if ('searchReady' in select.dataset) { return; } // set up once only
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
            // The editor redraws itself: the search box of the same list (by
            // its name) takes the focus again, for the next pick.
            refocus = select.name;
            select.dispatchEvent(new Event('change', { bubbles: true }));
        };
        let asked = []; // option values the search URL found for the query, in order
        let askedFor = '';
        const ask = async () => {
            const url = select.dataset.searchUrl;
            const q = input.value.trim();
            if (!url || q === '' || q === askedFor) { return; }
            askedFor = q;
            try {
                const response = await fetch(`${url}?q=${encodeURIComponent(q)}`);
                const found = response.ok ? await response.json() : [];
                if (input.value.trim() === q) { asked = found; show(); }
            } catch (e) { /* no signal: the labels' matches stand */ }
        };
        let timer;
        const matching = () => {
            const words = plain(input.value).split(/\s+/).filter(Boolean);
            const byLabel = options().filter((x) => words.every((w) => x.text.includes(w)));
            const bySearch = asked.map((v) => options().find((x) => x.o.value === v)).filter((x) => x && !byLabel.includes(x));
            return [...byLabel, ...bySearch];
        };
        const show = () => {
            const words = plain(input.value).split(/\s+/).filter(Boolean);
            if (words.length === 0) { list.hidden = true; return; }
            const found = matching().slice(0, most);
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
        input.addEventListener('input', () => {
            if (input.value.trim() !== askedFor) { asked = []; }
            show();
            clearTimeout(timer);
            timer = setTimeout(ask, 200);
        });
        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                const first = matching()[0];
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

    let refocus = ''; // the list just picked from, to focus again once redrawn
    const run = (root) => {
        (root.querySelectorAll ? root : document).querySelectorAll('select[data-search-select]').forEach(enhance);
        if (refocus) {
            const select = document.querySelector(`select[data-search-select][name="${CSS.escape(refocus)}"]`);
            const box = select && (select.closest('.select') || select).previousElementSibling;
            const input = box?.classList.contains('select-search') ? box.querySelector('.select-search-input') : null;
            if (input) { input.focus({ preventScroll: true }); refocus = ''; }
        }
    };
    if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', () => run(document)); } else { run(document); }
    document.addEventListener('htmx:load', (event) => run(event.detail.elt.parentElement || document));
})();
