// feedback.js: sends the Feedback form at the foot of a competition's pages
// (views/feedback.templ) without leaving the page, so nothing typed on it is
// lost, and says it's sent there. Without it, the form posts and comes back
// to the page.
(() => {
    for (const form of document.querySelectorAll('form[data-feedback]')) {
        const said = form.querySelector('[data-feedback-said]');
        form.addEventListener('submit', async (event) => {
            event.preventDefault();
            const button = form.querySelector('button[type="submit"]');
            button.disabled = true;
            said.textContent = 'Sending…';
            try {
                const response = await fetch(form.action, {
                    method: 'POST',
                    body: new URLSearchParams(new FormData(form)),
                    headers: { 'X-Feedback': 'fetch' },
                });
                said.textContent = await response.text();
                if (response.ok) {
                    form.querySelector('textarea').value = '';
                }
            } catch {
                said.textContent = "Couldn't send it: check your signal and try again.";
            } finally {
                button.disabled = false;
            }
        });
    }
})();
