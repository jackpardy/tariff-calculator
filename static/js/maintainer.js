// maintainer.js: the maintainer's page of feedback (views/feedback.templ,
// Maintainer). A copy of the page someone sent is shown, as its section
// opens, in a sandboxed frame given the copy's HTML (srcdoc): nothing in it
// runs, sends a form or goes anywhere, and the frame asks nothing of the
// server itself, which a browser may refuse a sandboxed frame. The frame is
// the page's whole height, in a box the size of their window, scrolled to
// where they were.
(() => {
    for (const frame of document.querySelectorAll('iframe[data-copy]')) {
        const box = frame.parentElement;
        const opens = frame.closest('details');
        let asked = false;
        const load = () => {
            if (asked) {
                return;
            }
            asked = true;
            fetch(frame.dataset.copy)
                .then((r) => (r.ok ? r.text() : Promise.reject(new Error(r.status))))
                .then((html) => {
                    frame.srcdoc = html;
                    // The frame's height is set, so the box can scroll now.
                    box.scrollTop = Number(box.dataset.scroll) || 0;
                })
                .catch(() => {
                    asked = false;
                    box.textContent = "Couldn't load the copy: open it on its own.";
                });
        };
        if (!opens || opens.open) {
            load();
        } else {
            opens.addEventListener('toggle', () => opens.open && load());
        }
    }
})();
