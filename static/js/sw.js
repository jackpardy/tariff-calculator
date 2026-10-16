// sw.js: the service worker for the competition tools. It shows push
// notifications (ADR 0008 Decision 3) and opens the page each is about when
// tapped, and keeps a person's pages on their phone so they open offline
// (below). Served at /sw.js, so it covers the whole site.
self.addEventListener('push', (event) => {
    let m = {};
    try {
        m = event.data ? event.data.json() : {};
    } catch (e) {
        m = { body: event.data ? event.data.text() : '' };
    }
    event.waitUntil(self.registration.showNotification(m.title || 'Competition changes', {
        body: m.body || '',
        data: { url: m.url || '/' },
    }));
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    let url = (event.notification.data && event.notification.data.url) || '/';
    if (new URL(url, self.location.origin).origin !== self.location.origin) {
        url = '/';
    }
    event.waitUntil(self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((open) => {
        for (const c of open) {
            if (c.url === url && 'focus' in c) {
                return c.focus();
            }
        }
        return self.clients.openWindow(url);
    }));
});

// Offline pages. The competition tools are used at venues with poor wifi, so
// a person's own pages are kept on their phone and open as last seen when the
// connection drops. Nothing leaves the phone: the copies live in this
// browser's Cache Storage, for this site only.
//
// The pages are server-rendered and sent with Cache-Control: no-store. That
// stops the browser's HTTP cache keeping them, but the Cache API ignores it,
// so a copy put here is kept anyway. That is meant: they are the person's own
// secret-link pages, on their own phone.
const PAGES = 'pages-v1';
const STATIC = 'static-v1';
const MAX_PAGES = 60;
// A copy older than this is dropped, so a phone doesn't keep an admin or
// helper page for good.
const MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000;
const NETWORK_WAIT_MS = 4000;
const OFFLINE_PAGE = '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">' +
    '<meta name="viewport" content="width=device-width, initial-scale=1"><title>Offline</title></head>' +
    '<body style="font-family: sans-serif; max-width: 30rem; margin: 3rem auto; padding: 0 1rem">' +
    "<h1>You're offline</h1><p>You're offline, and this page hasn't been opened on this phone before.</p></body></html>";

self.addEventListener('install', () => {
    self.skipWaiting();
});

// activate takes over open pages at once and drops caches left by older
// versions of this file (any not named above), and pages kept too long.
self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys()
            .then((keys) => Promise.all(keys.filter((k) => k !== PAGES && k !== STATIC).map((k) => caches.delete(k))))
            .then(() => caches.open(PAGES))
            .then((cache) => trimPages(cache))
            .then(() => self.clients.claim()));
});

// tooOld is whether a kept page is past MAX_AGE_MS, by the Date the server
// sent it with. A copy without one is kept to MAX_PAGES like any other.
function tooOld(response) {
    const sent = Date.parse(response.headers.get('Date') || '');
    return !Number.isNaN(sent) && Date.now() - sent > MAX_AGE_MS;
}

// isPage is whether a path is one of the competition tools' pages (admin pages
// included). The calculator's own pages and its APIs are left to the network.
function isPage(path) {
    return path.startsWith('/clubs/') || path.startsWith('/competitions/') || path.startsWith('/notify/');
}

// trimPages keeps the newest MAX_PAGES copies (keys come back oldest first),
// and drops any older than MAX_AGE_MS.
async function trimPages(cache) {
    const keys = await cache.keys();
    const over = Math.max(0, keys.length - MAX_PAGES);
    for (const [i, key] of keys.entries()) {
        const kept = i < over ? null : await cache.match(key);
        if (!kept || tooOld(kept)) {
            await cache.delete(key);
        }
    }
}

// pageFromNetwork fetches a page, giving up if the server hasn't started
// answering within NETWORK_WAIT_MS, so that a poor connection falls back to the
// copy at once. Once it has, the page comes through however slowly, as it would
// without this worker. Only good, same-origin responses are kept, and only once
// the whole page has arrived (see keep). A page the server says isn't there
// (404) or is gone (410), such as a replaced or deleted link's, drops its copy.
async function pageFromNetwork(request, event) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), NETWORK_WAIT_MS);
    let response;
    try {
        response = await fetch(request, { signal: controller.signal });
    } finally {
        clearTimeout(timer);
    }
    if (response.ok && response.type === 'basic') {
        event.waitUntil(keep(request.url, response.clone()).catch(() => {}));
    } else if (response.status === 404 || response.status === 410) {
        event.waitUntil(caches.open(PAGES).then((cache) => cache.delete(request.url)));
    }
    return response;
}

// keep puts a page in the cache once all of it has arrived, replacing the older
// copy (deleted then, so the new one counts as the newest). If the page never
// finishes arriving, the older copy stays.
async function keep(url, response) {
    const body = await response.blob();
    const cache = await caches.open(PAGES);
    await cache.delete(url);
    await cache.put(url, new Response(body, {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
    }));
    await trimPages(cache);
}

// page is network first: the live page when the connection answers in time,
// else the copy last seen (unless it's too old), else a short page saying
// there isn't one.
async function page(request, event) {
    try {
        return await pageFromNetwork(request, event);
    } catch (e) {
        const cache = await caches.open(PAGES);
        let cached = await cache.match(request.url);
        if (cached && tooOld(cached)) {
            await cache.delete(request.url);
            cached = undefined;
        }
        if (cached) {
            // Show the page's "you're offline" note: on a slow connection the
            // phone still counts as online, so the page itself wouldn't.
            const html = (await cached.text()).replace(' comp-offline" hidden>', ' comp-offline" data-from-copy>');
            return new Response(html, { headers: cached.headers });
        }
        return new Response(OFFLINE_PAGE, {
            status: 503,
            headers: { 'Content-Type': 'text/html; charset=utf-8' },
        });
    }
}

// asset is cache first: static URLs carry a version (?v=hash), so a cached one
// is never stale.
async function asset(request) {
    const cache = await caches.open(STATIC);
    const cached = await cache.match(request);
    if (cached) {
        return cached;
    }
    const response = await fetch(request);
    if (response.ok && response.type === 'basic') {
        await cache.put(request, response.clone());
    }
    return response;
}

self.addEventListener('fetch', (event) => {
    const request = event.request;
    if (request.method !== 'GET') {
        return;
    }
    const url = new URL(request.url);
    if (url.origin !== self.location.origin) {
        return;
    }
    if (request.mode === 'navigate' && isPage(url.pathname)) {
        event.respondWith(page(request, event));
    } else if (url.pathname.startsWith('/static/')) {
        event.respondWith(asset(request));
    }
    // Anything else (the calculator's pages and APIs) goes to the network.
});
