// The "How it's judged" pop-up. htmx adds it to the end of the page; this
// closes it (the ✕, the background or Escape), keeps only one open, and puts
// focus back on the button that opened it.
(() => {
    let opener = null;

    function close() {
        document.querySelectorAll('.judging-modal').forEach(m => m.remove());
        if (opener && document.contains(opener)) opener.focus();
        opener = null;
    }

    document.addEventListener('htmx:beforeRequest', e => {
        if (!e.detail.elt.hasAttribute('data-judging')) return;
        document.querySelectorAll('.judging-modal').forEach(m => m.remove());
        opener = e.detail.elt;
    });
    document.addEventListener('htmx:afterSwap', () => {
        const modal = document.querySelector('.judging-modal');
        if (modal) modal.querySelector('.delete')?.focus();
    });
    document.addEventListener('click', e => {
        if (e.target.closest('[data-judging-close]')) close();
    });
    document.addEventListener('keydown', e => {
        if (e.key === 'Escape' && document.querySelector('.judging-modal')) close();
    });
})();
