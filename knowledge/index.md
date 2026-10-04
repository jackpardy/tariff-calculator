---
okf_version: "0.2"
---

# Tariff calculator knowledge

An [Open Knowledge Format](https://github.com/GoogleCloudPlatform/open-knowledge-format)
bundle describing the trampoline tariff calculator: the trampolining rules it
implements, the requirements it checks routines against, its features, how it is
built and how it is deployed. Start with the [overview](overview.md).

* [Overview](overview.md) - What the app is, who uses it, and how the pieces fit together.
* [Glossary](glossary.md) - Trampolining and app terms, including why requirement lists are never called "sets".
* [Open questions](open-questions.md) - Rules, documents and checks still waiting on someone.

# Sections

* [Domain](domain/) - The skill model, the Code of Points tariff, routine validation and the skill catalog.
* [Requirements](requirements/) - Checking a routine against a competition's rules, and the built-in requirements.
* [Features](features/) - What a coach can do: build routines, share them, print a tariff sheet, compare two.
* [Architecture](architecture/) - Go, templ, htmx and Alpine; server rendering; routes, assets and tests.
* [Operations](operations/) - CI, Render, and the move to the self-hosted server.
