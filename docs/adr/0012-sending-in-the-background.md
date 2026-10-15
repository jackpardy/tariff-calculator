# ADR 0012 — Sending in the background

- **Status:** Accepted; built 2026-10-15
- **Date:** 2026-10-15
- **Deciders:** jackpardy (solo maintainer)
- **Amends:** ADR 0008 (notifications), on how emails and pushes are sent

## Context

A desk message emailed and pushed each address one at a time inside the
organiser's request (`tellDesk`). Over SMTP to Amazon SES an email takes
about half a second to a second, so a message to 300 people held the page
for minutes, could time out part way (the server's write timeout is 10
seconds) and left the organiser not knowing what went. Change notifications
already ran outside requests, in the notifier's loop, but also one at a
time, so a burst queued up behind slow sends. SES allows 14 emails a second
once out of its sandbox (1 a second in it), so sending faster needs a cap.

This is for the late-2026 dry run (about 20 people) and ISTO in April 2027
(a few hundred), on one small server.

## Decision

1. **One background sender** (`web/sender.go`) for desk messages, Notify
   and change notifications. The request (or the notifier's loop) works out
   who is told exactly as before, one delivery per address, hands the
   deliveries over and carries on. The desk answers at once with how many
   it's sending to, by email and by push.
2. **Four workers, emails capped a second.** Four deliveries go at once;
   each email waits for its slot under `MAIL_PER_SECOND` (default 14).
   Pushes go to the browsers' push services and aren't capped.
3. **One retry.** A failed email is put back once, after 5 seconds, then
   counted as failed and logged. A push service saying the phone is gone
   drops the subscription, as before; other push failures aren't retried.
4. **Counts on the desk.** While a message is sending, its counts (told by
   push, by email, failed, still to go) are in memory; when it's done
   they're written to `desk_messages` (`pushed`, `emailed`, and a new
   `failed`, migration 41). The desk page shows them when it's reloaded.
5. **The queue is in memory.** On SIGTERM, after the requests under way
   finish, the app waits up to 20 seconds for what's queued, then stops,
   logs how many were left unsent and keeps those as failed for their
   desk messages.
6. **The confirmation email stays in the request.** It's one email, and its
   failure is shown on the form (the subscription is removed so it can be
   tried again). It doesn't take a slot under the cap; the per-address and
   per-competition limits on confirmations keep it rare.

## Alternatives considered

- **A queue in the database** (an outbox table the workers read), so a
  restart loses nothing. Rejected for now: a deploy waits for the queue
  instead, and at the dry run's and ISTO's sizes a message drains in well
  under the 20 seconds; a row per delivery would also add a write per
  email to the same SQLite database the day's pages use. Worth revisiting
  if messages grow to thousands or deploys happen during a competition.
- **Sending all at once, one goroutine per address.** Rejected: it would
  break SES's rate, and the failures would come back as throttling.
- **Waiting in the request for a faster send** (four at a time but still
  answering when done). Rejected: 300 emails at 14 a second is still over
  20 seconds, past the write timeout.

## Consequences

- The organiser's page answers in well under a second whatever the size of
  the message; "Sending to 214 people: 180 by email, 34 by push." replaces
  "Told 34 by push and 180 by email".
- A restart during a send can lose deliveries after the 20-second wait;
  the log says how many, and the desk shows them as failed.
- The server's Compose file must give the app time to stop: a
  `stop_grace_period` of at least 40 seconds (15 for requests, 20 for the
  queue).
- Tests wait for the sender (`flush`) rather than sleeping: the test kit's
  handler flushes after each request, and `tellDue` after the notifier runs.
