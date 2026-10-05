---
okf_version: "0.2"
---

# Tariff calculator knowledge

An [Open Knowledge Format](https://github.com/GoogleCloudPlatform/open-knowledge-format)
bundle describing the trampoline tariff calculator: the trampolining rules it
implements, the requirements it checks routines against, its features, how it is
built and how it is deployed. Start with the [overview](overview.md).

* [Overview](overview.md) - What the app is, who uses it, and how the pieces fit together.
* [Features](guide/features.md) - Everything the app can do, in plain language, for gymnasts and coaches.
* [User guide](guide/user-guide.md) - Step by step: build a routine, check it against a competition, work on a level, share, view and print.
* [Glossary](glossary.md) - Trampolining and app terms, including why requirement lists are never called "sets".
* [Roadmap](roadmap.md) - Where the project could go next: tools for competition organisers and attendees, and for clubs, and what to build first.
* [Open questions](open-questions.md) - Rules, documents and checks still waiting on someone.

# Sections

* [Domain](domain/) - The skill model, the Code of Points tariff, routine validation and the skill catalog.
* [Requirements](requirements/) - Checking a routine against a competition's rules, and the built-in requirements.
* [Guide](guide/) - For gymnasts and coaches: what the app does and how to use it.
* [Features](features/) - How each feature works: the routine builder, sharing, the view screen, the tariff sheet, comparing.
* [Architecture](architecture/) - Go, templ, htmx and Alpine; server rendering; routes, assets and tests.
* [Operations](operations/) - CI, Render, and the move to the self-hosted server.
