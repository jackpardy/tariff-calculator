// taps.js: taps on the On the day page (views/day.templ) kept on the phone
// while there's no signal. The venue's wifi drops, so each tap (Started,
// Finished, Next gymnast, Here, Scratched, an official's check-in, Still
// here: the forms marked data-tap) is kept in this browser with the time it
// was tapped, shown on the page at once as waiting to send, and sent in
// order: straight away, when the signal comes back, when the page loads and
// every half minute. Each goes with the phone's clock when tapped and when
// sent, so the server can put it on its own clock (web/taps.go).
// competitions.js (dayPage) sends the day page's taps through here.
const DayTaps = (function () {
    const KEY = 'trampolineTaps';
    // A tap the server keeps failing on (an error page, not a lost signal)
    // is given up after this many tries, so it can't hold up the rest.
    const MAX_TRIES = 5;

    // queue keeps taps in storage (localStorage on the phone) and sends them
    // with post(url, body), which gives fetch's Response or throws without a
    // signal; now is the phone's clock, in milliseconds.
    function queue(storage, post, now) {
        const load = () => {
            try {
                const list = JSON.parse(storage.getItem(KEY) || '[]');
                return Array.isArray(list) ? list.filter((t) => t && t.id && t.action) : [];
            } catch (e) {
                return [];
            }
        };
        const save = (list) => {
            try {
                storage.setItem(KEY, JSON.stringify(list));
                return true;
            } catch (e) {
                return false;
            }
        };

        // add keeps a tap ({action, body: the form URL-encoded, form: its
        // data-tap, value and label: the button tapped, up: who's up after
        // it}), with when it was tapped; false if it couldn't be kept.
        const add = (tap) => {
            const tapped = now();
            const body = new URLSearchParams(tap.body);
            body.set('tapped', String(tapped));
            const list = load();
            list.push({ ...tap, body: body.toString(), tapped, tries: 0, id: tapped + '-' + Math.random().toString(36).slice(2) });
            return save(list);
        };

        // flush sends the taps kept, oldest first, one at a time, each with
        // when it's sent. A tap the server answered is dropped, whatever the
        // answer: done, done already, or refused (it says why). Without a
        // signal it stops, keeping the rest for next time. answers are the
        // server's answers, in order; stopped says whether any are left.
        const flush = async () => {
            const answers = [];
            for (;;) {
                const tap = load()[0];
                if (!tap) {
                    return { answers, stopped: false };
                }
                const body = new URLSearchParams(tap.body);
                body.set('sent', String(now()));
                let response;
                let text;
                try {
                    response = await post(tap.action, body);
                    text = await response.text();
                } catch (e) {
                    return { answers, stopped: true };
                }
                // Read again, as another tab may have sent some meanwhile.
                const list = load();
                const kept = list.find((t) => t.id === tap.id);
                if (response.status >= 500 && kept && ++kept.tries < MAX_TRIES) {
                    save(list);
                    return { answers, stopped: true };
                }
                save(list.filter((t) => t.id !== tap.id));
                answers.push({ tap, status: response.status, url: response.url, text });
            }
        };
        return { load, add, flush };
    }

    // clock is a time on the phone's clock, as the page shows times: "10:05".
    function clock(ms) {
        const d = new Date(ms);
        return String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
    }

    // upOrder is a running flight's routines in order, as who's up goes
    // through them (competitions.UpOrder): everyone's first routine, then
    // everyone's second, from the flight's check-in rows. scratched says
    // whether an entry is scratched (waiting taps count).
    function upOrder(box, scratched) {
        const rows = [...box.querySelectorAll('.comp-checkin-row[data-entry]')];
        const out = [];
        for (const round of [1, 2]) {
            for (const row of rows) {
                out.push({
                    entry: row.dataset.entry, round: String(round), no: row.dataset.no, name: row.dataset.name || '',
                    club: row.dataset.club || '', routine: (row.dataset.routines || '').split('|')[round - 1] || '',
                    open: !scratched(row.dataset.entry, row.dataset.state === 'scratched'),
                });
            }
        }
        return out;
    }

    // upStep is the routine anyone competes at after (step 1) or before
    // (step -1) the one with this entry and round ("" before the first tap),
    // as UpOrder's Next and Back; null if there's none.
    function upStep(order, entry, round, step) {
        let i = order.findIndex((r) => r.entry === entry && r.round === round);
        if (i < 0) {
            if (step < 0) { return null; }
            // Before the first tap, the first; from someone moved out, the
            // first in their round.
            i = entry === '' ? -1 : (Math.max(Number(round) || 1, 1) - 1) * (order.length / 2) - 1;
        }
        for (i += step; i >= 0 && i < order.length; i += step) {
            if (order[i].open) { return order[i]; }
        }
        return null;
    }

    // showUp puts a gymnast in a flight's who's up box, as the server would,
    // and in its form, so the next tap goes on from them.
    function showUp(up, g) {
        // On the last routine the box holds the flight's form (Finished) too:
        // this is the who's-up form.
        const form = up.querySelector('form[data-tap^="up:"]');
        if (!form) { return; }
        for (const p of [...up.children].filter((el) => el.tagName === 'P' && el !== form)) { p.remove(); }
        const label = Object.assign(document.createElement('p'), { className: 'is-size-7 has-text-grey mb-0', textContent: 'On now' });
        const name = Object.assign(document.createElement('p'), { className: 'is-size-4 has-text-weight-bold mb-0' });
        if (g.no) {
            name.append(Object.assign(document.createElement('span'), { className: 'comp-run-no', title: 'Running number', textContent: g.no }));
        }
        name.append(Object.assign(document.createElement('span'), { textContent: g.name }));
        const routine = Object.assign(document.createElement('p'), { className: 'mb-2' });
        if (g.club) {
            routine.append(Object.assign(document.createElement('span'), { className: 'has-text-grey', textContent: g.club + ' · ' }));
        }
        routine.append(Object.assign(document.createElement('strong'), { textContent: g.routine }));
        up.prepend(label, name, routine);
        form.querySelector('input[name="entry"]').value = g.entry;
        form.querySelector('input[name="round"]').value = g.round;
        button(form, 'back', 'Back', 'button is-light is-small').hidden = !g.canBack;
        // Back from the last routine: Next gymnast again, in Finished's place.
        button(form, 'next', 'Next gymnast', 'button is-link is-medium').hidden = !g.canNext;
        const finish = up.querySelector('form[data-tap^="flight:"]');
        if (finish) { finish.hidden = g.canNext; }
        if (!g.canNext) {
            up.append(Object.assign(document.createElement('p'), { className: 'is-size-7 mt-2', textContent: "The flight's last routine: tap Finished once it's done." }));
        }
    }

    // button finds a form's button for a value, making it if the page
    // doesn't have it (Finished, once Started is waiting to send).
    function button(form, value, label, className) {
        let b = form.querySelector(`button[name="what"][value="${value}"], button[name="status"][value="${value}"]`);
        if (!b) {
            b = Object.assign(document.createElement('button'), { className, type: 'submit', name: 'what', value, textContent: label });
            form.append(b);
        }
        return b;
    }

    // markWaiting shows the taps waiting to send on the day page as done:
    // each form's buttons as they'll be once it's sent, who's up moved on,
    // and a note with when it was tapped.
    function markWaiting(taps) {
        document.querySelectorAll('[data-tap-note]').forEach((n) => n.remove());
        const byForm = new Map();
        for (const tap of taps) {
            if (!byForm.has(tap.form)) { byForm.set(tap.form, []); }
            byForm.get(tap.form).push(tap);
        }
        for (const [key, list] of byForm) {
            const form = document.querySelector(`form[data-tap="${CSS.escape(key)}"]`);
            if (!form) { continue; }
            try { showWaiting(key, form, list); } catch (e) { console.error('Showing a tap waiting to send:', e); } // never in the way of sending
        }
    }

    // showWaiting shows taps kept on the phone on their form, as the server
    // will show them once sent.
    function showWaiting(key, form, list) {
        const last = list[list.length - 1];
        if (key.startsWith('flight:')) {
            markFlight(form, list);
        } else if (key.startsWith('up:')) {
            const g = list.map((t) => t.up).filter(Boolean).pop();
            if (g) { showUp(form.closest('.comp-up'), g); }
        } else {
            for (const b of form.querySelectorAll('button[type="submit"]')) { b.hidden = b.value === last.value; }
        }
        const note = Object.assign(document.createElement('p'), {
            className: 'help comp-tap-note',
            textContent: `${last.label} at ${clock(last.tapped)}: waiting to send` + (list.length > 1 ? ` (${list.length} taps)` : ''),
        });
        note.dataset.tapNote = '';
        form.after(note);
    }

    // markFlight shows a flight's Started, Finished and Undo as they'll be
    // once its waiting taps are sent, and who's up while it's running.
    function markFlight(form, list) {
        const box = form.closest('.comp-flight');
        const has = (v) => !!form.querySelector(`button[value="${v}"]`);
        // As the server last showed it, before any taps were marked here.
        form.dataset.shown ||= has('finish') ? 'running' : has('start') ? 'waiting' : 'finished';
        let state = form.dataset.shown;
        for (const tap of list) {
            state = { start: 'running', finish: 'finished', 'undo-finish': 'running', 'undo-start': 'waiting' }[tap.value] || state;
        }
        button(form, 'start', 'Started', 'button is-success').hidden = state !== 'waiting';
        button(form, 'finish', 'Finished', 'button is-success').hidden = state !== 'running';
        // Undo takes back what the page was showing, so it waits until then.
        form.querySelectorAll('button[value^="undo-"]').forEach((b) => { b.hidden = true; });
        const up = box?.querySelector('.comp-up');
        if (up) { up.hidden = state !== 'running'; }
    }

    // count says how many taps are waiting, at the top of the page.
    function count(n) {
        const el = document.getElementById('day-taps');
        if (!el) { return; }
        el.textContent = n === 0 ? '' : `Waiting to send: ${n === 1 ? '1 tap' : n + ' taps'}. They're kept on this phone and sent when the signal is back.`;
        el.hidden = n === 0;
    }

    // tapOf is what a tap on a form is to keep, or null if it isn't one:
    // the form (URL-encoded, with the button tapped), and for Next gymnast
    // and Back, who's up after it.
    function tapOf(form, submitter, waiting) {
        if (!form.dataset.tap) { return null; }
        const tap = {
            action: form.action, form: form.dataset.tap, body: new URLSearchParams(new FormData(form, submitter)).toString(),
            value: submitter?.value || '', label: submitter?.textContent.trim() || 'Tap',
        };
        const up = form.closest('.comp-up');
        if (up && (tap.value === 'next' || tap.value === 'back')) {
            // Scratched as the page shows, or as a tap waiting to send says.
            const scratched = (entry, shown) => {
                const t = waiting.filter((w) => w.form === 'checkin:' + entry).pop();
                return t ? t.value === 'scratched' : shown;
            };
            const order = upOrder(form.closest('.comp-flight') || document, scratched);
            const entry = form.querySelector('input[name="entry"]').value;
            const round = form.querySelector('input[name="round"]').value;
            const to = upStep(order, entry, round, tap.value === 'next' ? 1 : -1);
            if (to) {
                tap.up = { ...to, canBack: !!upStep(order, to.entry, to.round, -1), canNext: !!upStep(order, to.entry, to.round, 1) };
            }
        }
        return tap;
    }

    return { queue, markWaiting, count, tapOf, upOrder, upStep, clock };
})();
