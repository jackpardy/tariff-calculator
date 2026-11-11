// Runs taps.js's queue against a fake phone storage and server, to check the
// order taps are sent in, what's kept and what's dropped (web/taps_test.go
// runs it with node, if there is one): node tapstest.mjs path/to/taps.js
import fs from 'node:fs';
import vm from 'node:vm';

const ctx = { URLSearchParams, Math, JSON, String, Number, Array, Object, Date };
vm.createContext(ctx);
vm.runInContext(fs.readFileSync(process.argv[2], 'utf8') + '\nthis.DayTaps = DayTaps;', ctx);
const { DayTaps } = ctx;

let fails = 0;
const check = (what, cond) => { if (!cond) { fails++; console.log('FAIL', what); } else console.log('ok', what); };

const kept = new Map();
const storage = { getItem: (k) => kept.get(k) ?? null, setItem: (k, v) => kept.set(k, v) };
let clock = 1000;
let answer; // a function of the tap's body: a Response, or throws for no signal
const posted = [];
const q = DayTaps.queue(storage, async (url, body) => { posted.push({ url, body: new URLSearchParams(body) }); return answer(body); }, () => clock);

// Kept with when tapped; nothing sent without signal, in order once back.
answer = () => { throw new Error('offline'); };
q.add({ action: '/a/day/flight', body: 'flight=f&what=start', form: 'flight:f', value: 'start', label: 'Started' });
clock = 2000;
q.add({ action: '/a/day/checkin', body: 'entry=e&status=here', form: 'checkin:e', value: 'here', label: 'Here' });
let r = await q.flush();
check('without signal it stops, keeping both', r.stopped && q.load().length === 2 && r.answers.length === 0);
check('each is kept with when it was tapped', q.load()[0].tapped === 1000 && new URLSearchParams(q.load()[1].body).get('tapped') === '2000');

posted.length = 0;
clock = 600000;
answer = () => new Response('<div id="day-areas" data-day-page></div>', { status: 200 });
r = await q.flush();
check('sent in order once back', posted.map((p) => p.url).join() === '/a/day/flight,/a/day/checkin' && !r.stopped && q.load().length === 0);
check('each with when it was tapped and sent', posted[0].body.get('tapped') === '1000' && posted[0].body.get('sent') === '600000' && posted[0].body.get('what') === 'start');
check('the answers, in order', r.answers.length === 2 && r.answers[0].tap.label === 'Started');

// Refused (an error page): dropped, the answer kept to say why, the rest go on.
q.add({ action: '/a/day/flight', body: 'flight=gone&what=start', form: 'flight:gone', value: 'start', label: 'Started' });
q.add({ action: '/a/day/checkin', body: 'entry=e&status=scratched', form: 'checkin:e', value: 'scratched', label: 'Scratched' });
answer = (body) => new Response(body.get('flight') === 'gone' ? '<h1>Not a flight</h1>' : 'ok', { status: body.get('flight') === 'gone' ? 400 : 200 });
r = await q.flush();
check('a refused tap is dropped and the next goes on', q.load().length === 0 && r.answers.map((a) => a.status).join() === '400,200');

// The server failing: kept and tried again, but given up after five tries.
q.add({ action: '/a/day/up', body: 'flight=f&what=next', form: 'up:f', value: 'next', label: 'Next gymnast' });
answer = () => new Response('oops', { status: 500 });
for (let i = 1; i <= 4; i++) {
    r = await q.flush();
    check(`a server error keeps it (try ${i})`, r.stopped && q.load().length === 1);
}
r = await q.flush();
check('given up on the fifth', !r.stopped && q.load().length === 0 && r.answers[0].status === 500);

// No storage on the phone: not kept, so the page sends it at once.
const full = DayTaps.queue({ getItem: () => null, setItem: () => { throw new Error('full'); } }, async () => {}, () => 1);
check('a tap that can\'t be kept says so', full.add({ action: '/x', body: '', form: 'f' }) === false);

// Who's up without signal goes as the server's UpOrder: everyone's first
// routine, then their second, skipping the scratched.
const row = (entry, state) => ({ dataset: { entry, no: '1', name: entry, club: '', routines: 'Set|Voluntary', state } });
const box = { querySelectorAll: () => [row('a', ''), row('b', 'scratched'), row('c', '')] };
const order = DayTaps.upOrder(box, (entry, shown) => shown);
const at = (r) => r ? r.entry + r.round : 'none';
check('first, before any tap', at(DayTaps.upStep(order, '', '', 1)) === 'a1');
check('skipping the scratched', at(DayTaps.upStep(order, 'a', '1', 1)) === 'c1');
check('on to the second routines', at(DayTaps.upStep(order, 'c', '1', 1)) === 'a2');
check('back', at(DayTaps.upStep(order, 'a', '2', -1)) === 'c1');
check('nothing after the last', at(DayTaps.upStep(order, 'c', '2', 1)) === 'none');
check('nothing before the first', at(DayTaps.upStep(order, 'a', '1', -1)) === 'none');

// A flight's state on the phone: Started, then Warm-up over, then Finished;
// Next gymnast without Warm-up over moves it on to competing too, and Undo
// takes back the latest.
const st = (shown, values, upMoved) => DayTaps.flightState(shown, values.map((value) => ({ value })), upMoved);
check('Started: warming up', st('waiting', ['start'], false) === 'warming');
check('Warm-up over: competing', st('waiting', ['start', 'warm-up'], false) === 'competing');
check('Next gymnast without it: competing', st('warming', [], true) === 'competing');
check('Finished without it', st('warming', ['finish'], false) === 'finished');
check('Undo of Warm-up over: warming up again', st('competing', ['undo-warm-up'], false) === 'warming');
check('Undo of Started: waiting', st('warming', ['undo-start'], false) === 'waiting');

// The form's buttons follow it: Warm-up over while warming up, with
// Finished beside it less marked, and Next gymnast less marked too.
const classes = (...names) => {
    const set = new Set(names);
    return { contains: (c) => set.has(c), toggle: (c, on) => { if (on) { set.add(c); } else { set.delete(c); } } };
};
const btn = (value) => ({ value, hidden: false, classList: classes('button', 'is-primary') });
const next = btn('next');
const up = { hidden: true, querySelector: () => next };
const buttons = [btn('start'), btn('warm-up'), btn('finish'), btn('undo-start')];
const form = {
    dataset: {},
    querySelector: (sel) => buttons.find((b) => sel.includes(`value="${b.value}"`)) || null,
    querySelectorAll: () => buttons.filter((b) => b.value.startsWith('undo-')),
    closest: () => ({ querySelector: () => up }),
};
const shownBy = () => buttons.filter((b) => !b.hidden).map((b) => b.value).join();
form.dataset.shown = 'waiting';
DayTaps.markFlight(form, [{ value: 'start' }], false);
check('warming up: Warm-up over and Finished', shownBy() === 'warm-up,finish' && !buttons[2].classList.contains('is-primary') && !up.hidden && !next.classList.contains('is-primary'));
DayTaps.markFlight(form, [{ value: 'start' }, { value: 'warm-up' }], false);
check('competing: Finished, marked', shownBy() === 'finish' && buttons[2].classList.contains('is-primary') && next.classList.contains('is-primary'));
DayTaps.markFlight(form, [{ value: 'start' }], true);
check('Next gymnast waiting: competing', shownBy() === 'finish');

// A kept Warm-up over goes with when it was tapped.
posted.length = 0;
clock = 700000;
answer = () => { throw new Error('offline'); };
q.add({ action: '/a/day/flight', body: 'flight=f&what=warm-up', form: 'flight:f', value: 'warm-up', label: 'Warm-up over' });
await q.flush();
clock = 760000;
answer = () => new Response('ok', { status: 200 });
await q.flush();
check('a kept Warm-up over is sent with its time', posted.length === 2 && posted[1].body.get('what') === 'warm-up' &&
    posted[1].body.get('tapped') === '700000' && posted[1].body.get('sent') === '760000' && q.load().length === 0);

if (fails > 0) {
    process.exit(1);
}
