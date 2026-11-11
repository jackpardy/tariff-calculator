// Runs competitions.js on a fake page and phone storage, to check which saved
// links a member's page replaces, how two of one name are told apart, and what
// a club's join page offers (web/joinagain_test.go runs it with node, if there
// is one): node linkstest.mjs path/to/competitions.js
import fs from 'node:fs';
import vm from 'node:vm';

let fails = 0;
const check = (what, cond) => { if (!cond) { fails++; console.log('FAIL', what); } else console.log('ok', what); };
const source = fs.readFileSync(process.argv[2], 'utf8');

// elem is a fake element; data is its dataset, flags its boolean attributes.
const elem = (data = {}, flags = []) => ({
    dataset: data, hidden: false, children: [], href: '', textContent: '', className: '',
    hasAttribute: (a) => flags.includes(a),
    append(...c) { this.children.push(...c); },
    querySelector(sel) { return this.inner?.[sel] ?? null; },
    querySelectorAll() { return []; },
    addEventListener() {}, closest() { return null; },
});

// open runs the script on a page that has these elements (by selector), over
// the phone's storage, and returns the storage and the elements.
function open(selectors, stored = {}, path = '/') {
    const store = new Map(Object.entries(stored).map(([k, v]) => [k, JSON.stringify(v)]));
    const listeners = {};
    const document = {
        querySelector: (s) => selectors[s] ?? null,
        querySelectorAll: () => [],
        getElementById: () => null,
        createElement: () => elem(),
        addEventListener: (e, f) => { listeners[e] = f; },
        documentElement: elem(), body: elem(),
    };
    const ctx = {
        document, URL, URLSearchParams, JSON, Object, Array, Set, Map, String, Number, Math, Date, Promise, console,
        localStorage: { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, v) },
        location: { pathname: path, origin: 'http://x', href: 'http://x' + path, hash: '' },
        navigator: {}, window: {}, setTimeout, clearTimeout,
        Option: function (text, value) { return { text, value }; },
    };
    ctx.window = ctx;
    vm.createContext(ctx);
    vm.runInContext(source, ctx);
    listeners.DOMContentLoaded();
    return { saved: (k) => JSON.parse(store.get(k) || '[]') };
}

const key = 'trampolineMemberLinks';
const memberPage = (url, who, club = 'club1', name = 'Aoife Byrne') =>
    ({ '[data-member-link]': elem({ memberLink: url, memberName: name + ' · UCD', memberPerson: name, memberClub: club, memberWho: who }) });

// A member's page saves its link, with the club and who it's for.
let phone = open(memberPage('http://x/clubs/member/a', 'w1'));
let links = phone.saved(key);
check('saved with club and person', links.length === 1 && links[0].club === 'club1' && links[0].who === 'w1' && links[0].person === 'Aoife Byrne');

// The same person's new link (the comp sec gave one) replaces the old.
phone = open(memberPage('http://x/clubs/member/a2', 'w1'), { [key]: links });
links = phone.saved(key);
check('a new link for the same member replaces the old', links.length === 1 && links[0].url.endsWith('/a2'));

// Another person of the same name is kept, with the older still there, told apart.
phone = open(memberPage('http://x/clubs/member/b', 'w2'), { [key]: links });
links = phone.saved(key);
check('another member of the same name is kept too', links.length === 2 && links.some((l) => l.url.endsWith('/a2')) && links.some((l) => l.url.endsWith('/b')));
check('told apart', new Set(links.map((l) => l.name)).size === 2 && links[0].name === 'Aoife Byrne · UCD (2)');

// Opening either again changes nothing.
phone = open(memberPage('http://x/clubs/member/b', 'w2'), { [key]: links });
check('opening the second again keeps both, names as they were', phone.saved(key).length === 2 && phone.saved(key)[0].name === 'Aoife Byrne · UCD (2)');
phone = open(memberPage('http://x/clubs/member/a2', 'w1'), { [key]: phone.saved(key) });
check('opening the first again keeps both, its own name', phone.saved(key).length === 2 && phone.saved(key)[0].name === 'Aoife Byrne · UCD');

// A link saved before clubs were kept with it is kept, and filled in when opened.
const old = [{ name: 'Aoife Byrne · UCD', url: 'http://x/clubs/member/old', coach: false }];
phone = open(memberPage('http://x/clubs/member/c', 'w3', 'club1', 'Cara'), { [key]: old });
check('an older link is kept', phone.saved(key).length === 2);

// Several different people on one phone (a parent's children, a coach's members) are all kept.
phone = open(memberPage('http://x/clubs/member/p1', 'k1', 'club1', 'Aoife Byrne'), {});
phone = open(memberPage('http://x/clubs/member/p2', 'k2', 'club1', 'Brian Byrne'), { [key]: phone.saved(key) });
phone = open(memberPage('http://x/clubs/member/p3', 'k3', 'club1', 'Cara Byrne'), { [key]: phone.saved(key) });
check('different people at one club are all kept, by their names', phone.saved(key).map((l) => l.person).sort().join() === 'Aoife Byrne,Brian Byrne,Cara Byrne');

// The join page lists the pages saved at its club, and only those.
const join = (club, stored) => {
    const box = elem({ joinedHere: club });
    box.hidden = true; // as the page has it
    const list = elem();
    box.inner = { '[data-joined-list]': list };
    open({ '[data-joined-here]': box }, { [key]: stored });
    return { box, list };
};
const saved = [
    { name: 'Aoife Byrne · UCD', url: '/clubs/member/a', club: 'club1', who: 'w1', person: 'Aoife Byrne' },
    { name: 'Dan · Other', url: '/clubs/member/d', club: 'club2', who: 'w4', person: 'Dan' },
    { name: 'Old · UCD', url: '/clubs/member/o' },
];
let j = join('club1', saved);
check('the join page shows one page at this club, hidden otherwise', !j.box.hidden && j.list.children.length === 1);
const item = j.list.children[0];
check('with its name and a link to open it', item.children[0] === 'Aoife Byrne' && item.children[1].href === '/clubs/member/a' && item.children[1].textContent === 'Open');
j = join('club3', saved);
check('another club shows nothing', j.box.hidden && j.list.children.length === 0);
j = join('club1', []);
check('no saved links shows nothing', j.box.hidden);

process.exit(fails ? 1 : 0);
