// push.js: "On this phone" on the page for hearing about a competition's
// changes (views/notify.templ, ADR 0008). Registers the service worker
// (/sw.js), asks to show notifications, subscribes with the server's key
// and sends the subscription to the page, or says why it can't: on an
// iPhone, the page must be opened from the home screen first.
(function () {
    function decode(key) {
        const s = atob(key.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - key.length % 4) % 4));
        return Uint8Array.from(s, (c) => c.charCodeAt(0));
    }

    document.addEventListener('DOMContentLoaded', () => {
        const box = document.querySelector('[data-push-key]');
        if (!box) {
            return;
        }
        const status = box.querySelector('[data-push-status]');
        const button = box.querySelector('[data-push-on]');
        let phones = [];
        try { phones = JSON.parse(box.dataset.pushPhones || '[]'); } catch (e) { /* none */ }
        const say = (text) => { status.textContent = text; };

        const supported = 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
        const iphone = /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
        const standalone = window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
        if (!supported) {
            button.hidden = true;
            say(iphone && !standalone
                ? 'On an iPhone or iPad, add this page to your home screen first (Share, then Add to Home Screen), open it from there, and turn this on.'
                : "This browser can't show notifications.");
            return;
        }
        navigator.serviceWorker.register('/sw.js')
            .then((reg) => reg.pushManager.getSubscription())
            .then((sub) => {
                const mine = sub && phones.find((p) => p.endpoint === sub.endpoint);
                if (mine) {
                    button.hidden = true;
                    say('On for this phone.');
                    const row = box.querySelector('[data-phone="' + mine.id + '"]');
                    if (row) {
                        row.querySelector('[data-phone-name]').textContent = 'This phone';
                    }
                }
            })
            .catch(() => {});

        button.addEventListener('click', async () => {
            button.disabled = true;
            try {
                if (await Notification.requestPermission() !== 'granted') {
                    say("Notifications are blocked for this site: allow them in the browser's settings, then try again.");
                    return;
                }
                const reg = await navigator.serviceWorker.register('/sw.js');
                await navigator.serviceWorker.ready;
                const sub = (await reg.pushManager.getSubscription()) ||
                    await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: decode(box.dataset.pushKey) });
                const j = sub.toJSON();
                const form = new URLSearchParams({ action: 'push', endpoint: j.endpoint, p256dh: j.keys.p256dh, auth: j.keys.auth });
                box.querySelectorAll('input[name="pushTopic"]:checked').forEach((i) => form.append('topic', i.value));
                const res = await fetch(box.dataset.pushAction, { method: 'POST', body: form });
                if (res.redirected) {
                    location.href = res.url;
                    return;
                }
                const page = new DOMParser().parseFromString(await res.text(), 'text/html');
                const problems = [...page.querySelectorAll('.is-danger li')].map((li) => li.textContent.trim());
                say(problems.join(' ') || "That didn't work. Please try again in a minute.");
            } catch (e) {
                say("That didn't work: " + e.message);
            } finally {
                button.disabled = false;
            }
        });
    });
})();
