// Runs sw.js against fake caches and fetch, to check what it keeps and
// drops (web/offline_test.go runs it with node, if there is one):
// node swtest.mjs path/to/sw.js
import fs from 'node:fs';
import vm from 'node:vm';

const src = fs.readFileSync(process.argv[2], 'utf8');
const store = new Map(); // url -> Response
const cache = {
    async keys() { return [...store.keys()].map((u) => ({ url: u, toString() { return u; } })); },
    async match(k, opts) {
        const u = typeof k === 'string' ? k : k.url;
        let r = store.get(u);
        if (!r && opts && opts.ignoreSearch) {
            const path = new URL(u).pathname;
            for (const [key, v] of store) { if (new URL(key).pathname === path) { r = v; } }
        }
        return r ? r.clone() : undefined;
    },
    async put(u, r) { store.set(u, r); },
    async delete(k) { return store.delete(typeof k === 'string' ? k : k.url); },
};
const listeners = {};
let nextFetch;
let fetchDelay = 0; // fake ms before nextFetch answers (honours an abort)
const SCALE = 100; // timers run 100 times faster, so 4 s and 15 s are quick
const ctx = {
    self: { addEventListener: (n, f) => { listeners[n] = f; }, location: { origin: 'https://x' }, registration: {}, clients: { claim: async () => {} }, skipWaiting() {} },
    caches: { open: async () => cache, keys: async () => ['pages-v1'], delete: async () => true, match: async (u) => cache.match(u) },
    fetch: async (req, init) => {
        const f = nextFetch;
        if (fetchDelay) {
            await new Promise((resolve, reject) => {
                const t = setTimeout(resolve, fetchDelay / SCALE);
                init?.signal?.addEventListener('abort', () => { clearTimeout(t); reject(Object.assign(new Error('aborted'), { name: 'AbortError' })); });
            });
        }
        if (f instanceof Error) throw f;
        return f;
    },
    Response, URL, AbortController, setTimeout: (f, ms) => setTimeout(f, ms / SCALE), clearTimeout, Date, Promise, Number,
};
vm.createContext(ctx);
vm.runInContext(src, ctx);
// page runs sw.js's page with a fetch event, and waits for what it kept or
// dropped after answering.
const page = async (request) => {
    const waits = [];
    const r = await ctx.page(request, { waitUntil: (p) => waits.push(p) });
    await Promise.all(waits);
    return r;
};
const ok = (body, date) => { const r = new Response(body, { status: 200, headers: { Date: date.toUTCString(), 'Content-Type': 'text/html' } }); Object.defineProperty(r, 'type', { value: 'basic' }); return r; };
const day = 24 * 3600 * 1000;
let fails = 0;
const check = (what, cond) => { if (!cond) { fails++; console.log('FAIL', what); } else console.log('ok', what); };

// Kept, then served offline.
nextFetch = ok('<p class="x comp-offline" hidden>day</p>', new Date());
await page({ url: 'https://x/clubs/member/t/competitions/c' });
check('a page is kept', store.has('https://x/clubs/member/t/competitions/c'));
nextFetch = new Error('offline');
let r = await page({ url: 'https://x/clubs/member/t/competitions/c' });
check('served offline, marked as a copy', (await r.text()).includes('data-from-copy'));

// 404 and 410 drop it.
for (const status of [404, 410]) {
    store.set('https://x/clubs/admin/a', ok('admin', new Date()));
    nextFetch = new Response('gone', { status });
    await page({ url: 'https://x/clubs/admin/a' });
    check(status + ' drops the copy', !store.has('https://x/clubs/admin/a'));
}
// A 500 keeps it.
store.set('https://x/clubs/admin/a', ok('admin', new Date()));
nextFetch = new Response('oops', { status: 500 });
await page({ url: 'https://x/clubs/admin/a' });
check('a 500 keeps the copy', store.has('https://x/clubs/admin/a'));

// Too old: not served offline, and dropped.
store.set('https://x/competitions/admin/old', ok('old', new Date(Date.now() - 31 * day)));
nextFetch = new Error('offline');
r = await page({ url: 'https://x/competitions/admin/old' });
check('a 31-day-old copy is not served', r.status === 503 && !store.has('https://x/competitions/admin/old'));
store.set('https://x/competitions/admin/recent', ok('recent', new Date(Date.now() - 29 * day)));
r = await page({ url: 'https://x/competitions/admin/recent' });
check('a 29-day-old copy is served', r.status === 200);

// Trimmed when another is kept: old ones dropped, the newest 60 kept.
for (let i = 0; i < 70; i++) store.set('https://x/p' + i, ok('p', new Date()));
store.set('https://x/competitions/admin/old2', ok('old', new Date(Date.now() - 40 * day)));
nextFetch = ok('new', new Date());
await page({ url: 'https://x/competitions/new' });
check('old copies trimmed', !store.has('https://x/competitions/admin/old2'));
check('at most 60 kept, the oldest dropped', store.size <= 60 && !store.has('https://x/p0') && store.has('https://x/p69') && store.has('https://x/competitions/new'));

// A page that stops arriving partway leaves the older copy.
store.set('https://x/competitions/slow', ok('older', new Date()));
const broken = new Response(new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode('part')); c.error(new Error('dropped')); } }), { status: 200 });
Object.defineProperty(broken, 'type', { value: 'basic' });
nextFetch = broken;
r = await page({ url: 'https://x/competitions/slow' });
check('a page cut off partway keeps the older copy', store.has('https://x/competitions/slow') && (await store.get('https://x/competitions/slow').clone().text()) === 'older');

// activate trims too.
store.set('https://x/competitions/admin/old3', ok('old', new Date(Date.now() - 40 * day)));
let waited;
listeners.activate({ waitUntil: (p) => { waited = p; } });
await waited;
check('activate drops old copies', !store.has('https://x/competitions/admin/old3'));
// A new version of a static file drops the older ones of it, and no other.
store.set('https://x/static/js/app.js?v=old', ok('old', new Date()));
store.set('https://x/static/css/app.css?v=old', ok('css', new Date()));
nextFetch = ok('new', new Date());
const assetWaits = [];
r = await ctx.asset({ url: 'https://x/static/js/app.js?v=new' }, { waitUntil: (p) => assetWaits.push(p) });
await Promise.all(assetWaits);
check('a new version of a static file is kept', (await r.text()) === 'new' && store.has('https://x/static/js/app.js?v=new'));
check('the older version is dropped', !store.has('https://x/static/js/app.js?v=old'));
check('other files are left', store.has('https://x/static/css/app.css?v=old'));
// A page kept before a deploy asks for the older version of a file. Offline,
// it gets the kept newer one.
store.clear();
nextFetch = ok('css1', new Date());
await ctx.asset({ url: 'https://x/static/css/styles.css?v=1' }, { waitUntil: () => {} });
nextFetch = ok('css2', new Date());
const w2 = [];
await ctx.asset({ url: 'https://x/static/css/styles.css?v=2' }, { waitUntil: (p) => w2.push(p) });
await Promise.all(w2);
check('v=1 is dropped once v=2 is kept', !store.has('https://x/static/css/styles.css?v=1') && store.has('https://x/static/css/styles.css?v=2'));
nextFetch = new Error('offline');
r = await ctx.asset({ url: 'https://x/static/css/styles.css?v=1' }, { waitUntil: () => {} });
check('offline, the older version gets the kept newer one', (await r.text()) === 'css2');
let threw = false;
try { await ctx.asset({ url: 'https://x/static/css/never.css?v=1' }, { waitUntil: () => {} }); } catch (e) { threw = true; }
check('offline, a file never kept still fails', threw);

// A slow answer. No kept copy: wait past 4 s, and take the page if it
// arrives before 15 s.
store.clear();
nextFetch = ok('<p>fresh</p>', new Date());
fetchDelay = 8000;
r = await page({ url: 'https://x/competitions/new-page' });
check('a never-kept page waits past 4 s and gets the page', r.status === 200 && (await r.text()) === '<p>fresh</p>');
// Never arriving: the offline page, with Try again.
fetchDelay = 20000;
r = await page({ url: 'https://x/competitions/new-page2' });
const offlineHTML = await r.text();
check('after 15 s the offline page has Try again', r.status === 503 && offlineHTML.includes('Try again'));
// A kept copy: the 4 s fallback stays, and the banner says the page is slow.
store.set('https://x/competitions/kept', ok('<p class="x comp-offline" hidden>k</p>', new Date()));
nextFetch = ok('<p>fresh</p>', new Date());
fetchDelay = 8000;
r = await page({ url: 'https://x/competitions/kept' });
let html = await r.text();
check('a kept copy is shown after 4 s, marked as slow', html.includes('data-from-copy') && html.includes('data-from-slow'));
// A real failure is marked as a copy only.
fetchDelay = 0;
nextFetch = new Error('offline');
r = await page({ url: 'https://x/competitions/kept' });
html = await r.text();
check('a real failure is not marked as slow', html.includes('data-from-copy') && !html.includes('data-from-slow'));
// A kept copy yields to a page answering in time.
nextFetch = ok('<p>live</p>', new Date());
fetchDelay = 2000;
r = await page({ url: 'https://x/competitions/kept' });
check('a kept copy yields to a page answering within 4 s', (await r.text()) === '<p>live</p>');
process.exit(fails ? 1 : 0);
