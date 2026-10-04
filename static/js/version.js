// version.js: tells a page left open across a deploy that the app has been
// updated. The page carries the version it was served with (<meta
// name="app-version">); every response from the server carries the current one
// (X-App-Version). When they differ, a bar offers to refresh, as the page's
// scripts and styles no longer match what the server sends.
(() => {
    const pageVersion = document.querySelector('meta[name="app-version"]')?.content;
    let shown = false;

    function check(serverVersion) {
        if (shown || !pageVersion || !serverVersion || serverVersion === pageVersion) { return; }
        shown = true;
        const bar = document.createElement('div');
        bar.setAttribute('role', 'status');
        bar.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:2000;display:flex;gap:0.75rem;align-items:center;justify-content:center;flex-wrap:wrap;padding:0.6rem 1rem;background:#485fc7;color:#fff;font:600 0.95rem system-ui,sans-serif;box-shadow:0 2px 6px rgba(0,0,0,0.2)';
        const text = document.createElement('span');
        text.textContent = 'The app has been updated.';
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = 'Refresh';
        button.style.cssText = 'border:0;border-radius:6px;padding:0.35rem 0.9rem;background:#fff;color:#485fc7;font:inherit;cursor:pointer';
        button.addEventListener('click', () => location.reload());
        bar.append(text, button);
        document.body.append(bar);
    }

    // htmx requests...
    document.addEventListener('htmx:afterRequest', (event) => check(event.detail.xhr?.getResponseHeader('X-App-Version')));
    // ...and the page's own fetches.
    const originalFetch = window.fetch;
    window.fetch = async (...args) => {
        const response = await originalFetch(...args);
        check(response.headers.get('X-App-Version'));
        return response;
    };
})();
