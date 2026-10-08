// sw.js: the service worker for the competition tools' push notifications
// (ADR 0008 Decision 3). It shows each one, and opens the page it's about
// when tapped. Served at /sw.js, so it covers the whole site.
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
