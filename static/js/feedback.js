// feedback.js: the Feedback button on every page (views/feedback.templ).
// It sends what's written without leaving the page, so nothing typed on it
// is lost, with details about the browser and, as the maintainer chooses,
// a picture of the screen and a copy of the page: Full takes both as the
// button opens, before the form covers anything; Light takes only the
// picture, when they tick for it, with what's marked data-private blurred.
// The picture's library (modern-screenshot) loads only then. Without
// JavaScript, the form posts and comes back to the page.
(() => {
    const box = document.querySelector('[data-site-feedback]');
    const form = box && box.querySelector('form[data-feedback]');
    if (!form) {
        return;
    }
    const details = box.querySelector('details');
    const light = box.hasAttribute('data-light');
    const said = form.querySelector('[data-feedback-said]');
    const figure = form.querySelector('[data-feedback-picture]');
    const img = figure.querySelector('img');
    const caption = form.querySelector('[data-feedback-caption]');
    const tick = form.querySelector('[data-feedback-tick]');
    const send = form.querySelector('button[type="submit"]');

    // The page's script errors and failed requests, for the details: never
    // an address, which may hold a link.
    const errors = [];
    const note = (what) => {
        errors.push(new Date().toTimeString().slice(0, 8) + ' ' + String(what).slice(0, 200));
        if (errors.length > 10) {
            errors.shift();
        }
    };
    addEventListener('error', (e) => note(e.message || 'A script error'));
    addEventListener('unhandledrejection', (e) => note('Not handled: ' + ((e.reason && e.reason.message) || e.reason)));
    document.addEventListener('htmx:responseError', (e) => note('A request failed: ' + ((e.detail && e.detail.xhr && e.detail.xhr.status) || '')));
    document.addEventListener('htmx:sendError', () => note("A request couldn't be sent"));

    // The pages opened before this one in the tab, by their patterns.
    const page = form.elements.page.value;
    let recent = [];
    try {
        recent = JSON.parse(sessionStorage.getItem('feedbackPages') || '[]');
        if (!Array.isArray(recent)) {
            recent = [];
        }
        sessionStorage.setItem('feedbackPages', JSON.stringify([...recent, page].slice(-7)));
    } catch (e) {
        recent = [];
    }

    let opened = null; // where they were when it opened: scrolled to, dropdowns open
    let picture = null; // a promise of the picture, or of null
    let copy = null; // a promise of the copy of the page, or of null
    let noPicture = '';

    const ours = (node) => node === box || (node.nodeType === 1 && box.contains(node));

    const library = () => {
        if (window.modernScreenshot) {
            return Promise.resolve(window.modernScreenshot);
        }
        return new Promise((resolve, reject) => {
            const s = document.createElement('script');
            s.src = box.dataset.shot;
            s.onload = () => resolve(window.modernScreenshot);
            s.onerror = () => reject(new Error("the picture's script didn't load"));
            document.head.append(s);
        });
    };

    // takePicture draws what's on the screen (not the whole page), without
    // this button.
    const takePicture = () => library().then((shot) => shot.domToBlob(document.body, {
        type: 'image/jpeg',
        quality: 0.7,
        width: innerWidth,
        height: innerHeight,
        scale: Math.min(devicePixelRatio || 1, 2),
        backgroundColor: '#fff',
        timeout: 15000,
        style: { transform: `translate(${-scrollX}px, ${-scrollY}px)`, transformOrigin: '0 0', margin: '0' },
        filter: (node) => !ours(node),
        onCloneEachNode: (node) => {
            if (node.nodeType !== 1) {
                return;
            }
            // The picture is drawn from an SVG, which is XML: an attribute
            // with a colon (Alpine's x-bind:value, x-on:click) is a prefix
            // nobody declared, and makes the whole picture blank.
            for (const a of [...node.attributes]) {
                if (a.name.includes(':') && !/^(xml|xmlns|xlink)(:|$)/.test(a.name)) {
                    node.removeAttribute(a.name);
                }
            }
            if (light && node.hasAttribute('data-private')) {
                node.style.filter = 'blur(6px)';
            }
        },
    }));

    const capture = () => {
        figure.hidden = false;
        img.removeAttribute('src');
        caption.textContent = 'Taking a picture of the screen…';
        noPicture = '';
        picture = takePicture().then((blob) => {
            img.src = URL.createObjectURL(blob);
            caption.textContent = 'This picture goes with it.';
            return blob;
        }).catch((err) => {
            noPicture = String((err && err.message) || err).slice(0, 200);
            figure.hidden = true;
            return null;
        });
    };

    // copyPage is the page's HTML as it is: what's typed, ticked and picked
    // written into it, without its scripts or this button, gzipped where the
    // browser can.
    const copyPage = () => {
        const live = [...document.querySelectorAll('input, textarea, select')];
        const doc = document.documentElement.cloneNode(true);
        const copied = [...doc.querySelectorAll('input, textarea, select')];
        live.forEach((el, i) => {
            const c = copied[i];
            if (!c || ours(el)) {
                return;
            }
            if (el.type === 'password' || el.type === 'file') {
                c.removeAttribute('value');
            } else if (el.type === 'checkbox' || el.type === 'radio') {
                c.toggleAttribute('checked', el.checked);
            } else if (el.tagName === 'TEXTAREA') {
                c.textContent = el.value;
            } else if (el.tagName === 'SELECT') {
                [...el.options].forEach((o, j) => c.options[j] && c.options[j].toggleAttribute('selected', o.selected));
            } else if (el.type !== 'hidden') {
                c.setAttribute('value', el.value);
            }
        });
        doc.querySelectorAll('[data-site-feedback], script, link[href*="/feedback.css"]').forEach((el) => el.remove());
        const html = '<!DOCTYPE html>\n' + doc.outerHTML;
        if (!window.CompressionStream) {
            return Promise.resolve(new Blob([html], { type: 'text/html' }));
        }
        return new Response(new Blob([html]).stream().pipeThrough(new CompressionStream('gzip'))).blob();
    };

    const openDropdowns = () => [...document.querySelectorAll('details[open] > summary')]
        .filter((s) => !ours(s))
        .map((s) => s.textContent.replace(/\s+/g, ' ').trim().slice(0, 100))
        .filter(Boolean)
        .slice(0, 10);

    details.addEventListener('toggle', () => {
        if (!details.open) {
            return;
        }
        said.textContent = '';
        opened = { scroll: [Math.round(scrollX), Math.round(scrollY)], open: light ? [] : openDropdowns() };
        if (!light) {
            capture();
            copy = copyPage().catch(() => null);
        }
    });
    if (tick) {
        tick.addEventListener('change', () => {
            if (tick.checked) {
                capture();
            } else {
                picture = null;
                figure.hidden = true;
            }
        });
    }
    form.querySelector('[data-feedback-close]').addEventListener('click', () => {
        details.open = false;
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && details.open) {
            details.open = false;
        }
    });

    const queued = () => {
        try {
            const list = JSON.parse(localStorage.getItem('trampolineTaps') || '[]');
            return Array.isArray(list) ? list.length : 0;
        } catch (e) {
            return 0;
        }
    };
    const meta = (name) => {
        const m = document.querySelector(`meta[name="${name}"]`);
        return m ? m.content : '';
    };
    const browser = () => ({
        viewport: [innerWidth, innerHeight],
        screen: [screen.width, screen.height],
        ratio: devicePixelRatio || 1,
        browser: navigator.userAgent,
        language: navigator.language,
        online: navigator.onLine,
        installed: matchMedia('(display-mode: standalone)').matches || navigator.standalone === true,
        version: meta('app-version'),
        pageTime: meta('page-time'),
        scroll: opened ? opened.scroll : [Math.round(scrollX), Math.round(scrollY)],
        height: document.documentElement.scrollHeight,
        queued: queued(),
        errors: errors,
        recent: recent.slice(-6),
        open: opened ? opened.open : [],
        noPicture: noPicture,
    });

    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        send.disabled = true;
        said.textContent = 'Sending…';
        try {
            const body = new FormData(form);
            const shot = picture && (!tick || tick.checked) ? await picture : null;
            if (shot) {
                body.append('picture', shot, 'screen.jpg');
            }
            const page = copy ? await copy : null;
            if (page) {
                body.append('snapshot', page, 'page.html');
            }
            body.append('details', JSON.stringify(browser()));
            const response = await fetch(form.action, {
                method: 'POST',
                body,
                headers: { 'X-Feedback': 'fetch' },
            });
            said.textContent = await response.text();
            if (response.ok) {
                form.elements.text.value = '';
                if (tick) {
                    tick.checked = false;
                }
                figure.hidden = true;
                picture = copy = null;
            }
        } catch (e) {
            said.textContent = "Couldn't send it: check your signal and try again.";
        } finally {
            send.disabled = false;
        }
    });
})();
