---
name: doctor
description: "Check whether the ikigenba tool suite (embed, autotune, oauth, idgen, agentrepl, ralph) is installed, report versions, and install any that are missing when asked. Use when a workflow needs these binaries on PATH, when an install seems broken, or when the user asks to set up / check / install the tools."
---

# doctor

Diagnoses and (on request) installs the ikigenba command-line tools the skills
here depend on. It wraps `doctor.sh`, the self-contained checker/installer in
this skill's directory — the script is the source of truth for the tool list and
the install commands; this file says when and how to run it.

## The tools it manages

`embed`, `autotune`, `login`, `idgen`, `agentrepl`, `ralph` — each a prebuilt Go
binary published as a GitHub release under `github.com/ikigenba/<tool>`. Each
carries its own `install.sh`; the doctor just knows the roster and orchestrates.

## Diagnose (default, read-only)

Run the script with no arguments and report the table it prints:

    sh doctor.sh

Each row is `ok` (on PATH, with the version `<tool> -V` reports) or `MISSING`.
Diagnosis never changes anything. This is the right move whenever you need these
binaries present, or the user asks "what's installed / what's missing".

## Install (only when the user asks)

Installing runs each missing tool's own installer, which pipes a release
`install.sh` to `sh` and drops a binary into `${PREFIX:-$HOME/.local}/bin`.
**Treat that as an outward-facing action: do it only when the user has asked to
install, and show what will run first.** Never install proactively off a bare
diagnosis.

- Everything missing, with consent already given:

      sh doctor.sh --install --yes

- A specific tool:

      sh doctor.sh --install --yes embed

Run it non-interactively (`--yes`) once the user has agreed; without `--yes` the
script prompts per tool at the terminal. After installing, re-run `sh doctor.sh`
and report the confirming table.

## Notes

- **Version format varies.** Released binaries report `v0.1.0`; a locally-built
  one may show a git sha or `dev`. The doctor treats presence plus any version
  string as success, not a strict format. It does not enforce a minimum version.
- **PATH.** A tool can be installed yet absent from the table if its bin dir
  isn't on `PATH`. The per-tool installer warns about this; if a freshly
  installed tool still reads `MISSING`, check that `${PREFIX:-$HOME/.local}/bin`
  is on `PATH`.
- **Runtime credentials are out of scope.** The doctor checks that binaries are
  present, not that their secrets are set (e.g. `embed` needs a provider key at
  run time). A tool can be `ok` here and still fail at runtime for want of a
  credential.
