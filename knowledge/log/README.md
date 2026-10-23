# The log's new lines

A change doesn't edit [log.md](../log.md): every pull request used to add its
line at the same place there, so whichever merged second conflicted. It adds
a file here instead, named for the day and the change:

    knowledge/log/2026-10-23-ci-faster.md

holding its line (or lines), just as log.md has them:

    * **Update**: [CI and deploys](operations/deploy.md#ci) are faster ...

Links are as from log.md (`operations/deploy.md`, not `../operations/...`),
as the line ends up there.

After the change merges to master, the **Update log** workflow
(`.github/workflows/log.yml`) moves the lines into log.md, under their
dates, newest first, and deletes the files (`scripts/log-entries.py`). CI
checks the files on every pull request (`--check`): the name
`YYYY-MM-DD-what.md`, in lower case with hyphens, and each line
`* **Kind**: ...`.

The workflow pushes to master with `LOG_TOKEN`, a fine-grained token of the
repo's admin (Contents: read and write, this repository only), as master
only takes a push whose commit has passed "test", or an admin's. Without it
the lines wait here, and the workflow says so.
