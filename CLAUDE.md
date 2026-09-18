# CLAUDE.md


## How to work (high-level mindset)

The marginal cost of completeness is near zero with AI. Do the whole thing. Do it right. Do it with tests. Do it with documentation. Do it so well that KD is genuinely impressed — not politely satisfied, actually impressed. Never offer to "table this for later" when the permanent solve is within reach. Never leave a dangling thread when tying it off takes five more minutes. Never present a workaround when the real fix exists. The standard isn't "good enough" — it's "holy shit, that's done."

Search before building. Test before shipping. Ship the complete thing. When Julien asks for something, the answer is the finished product, not a plan to build it.

Time is not an excuse. Fatigue is not an excuse. Complexity is not an excuse. Boil the ocean. This is how we think about shipping.

You can outsource the typing. You cannot outsource the understanding. Before you call anything DONE you must be able to explain why the code is correct and exactly where it would break. Tests passing is not understanding. If you can't walk the failure modes out loud, you're not done, you're guessing.

## Branching - one branch per task
This section is non-negotiable and must never be removed. It runs first, before the triage block, because the triage block has to report the branch it produces.

Two facts hold at once: Julien works with other people, so nothing lands on main directly; and several Claude Code sessions run on the same machine, in the same repo, at the same time.

Branches should be named - "kd/<task-id>-<task-title>". 

Create branches from main branch only and always fetch latest main branch before creating a new branch for a task.

Multiple sessions may be started for the same task and should  continue on the branches created for that task. 


## Task sizing

Every task starts with a printed triage block, before any work. One exception, and only one: the setup block in "Branching" runs first, because the triage block reports the branch it creates. Four lines:

* Size: small | medium | large — why
* Tests: local (which ones) | full suite — why
* Branch: <branch name> see "Branching"

This block is mandatory and verbose on purpose. KD reads it to see what mode was picked and to tune these rules over time. A wrong mode is only correctable if the choice is visible. Never skip it, never bury it mid-report. The Branch line is there so that with several sessions running at once, KD can tell at a glance which one is about to touch what.


## Completion status protocol
At the end of every task, report one of:

* DONE — All steps completed. Evidence provided for every claim with tests + evals. Ready to merge.
* DONE_WITH_CONCERNS — Completed, but with issues Julien should know about. List each concern with severity and a proposed follow-up.
* BLOCKED — Cannot proceed. State what's blocking and what was already tried.
* NEEDS_CONTEXT — Missing information required to continue. State exactly what's needed.

"Partially done" is not a status. Either the feature ships (DONE) or it doesn't (BLOCKED / NEEDS_CONTEXT). Honesty about incompleteness beats pretending.


## Safety
* Never edit secrets.
* Never commit secrets. If .env is touched, verify .gitignore before any commit.
* Never skip pre-commit hooks with --no-verify. If a hook fails, fix the underlying issue.


## Development flow
- Define the task
- Create the branch
- Implement the change
- Commit the change to the branch
- Open a pull request


## Conventions

- Go version: see `go.mod` (currently 1.24).
- Format every change with `gofmt` before committing.
- Prefer the standard library; justify new dependencies in the PR
  description.
- Exported identifiers need Go doc comments; avoid comments that
  restate what the code already says.
- Follow [Keep a Changelog](https://keepachangelog.com/) conventions in
  `CHANGELOG.md`.
- Every line of code must have an English comment above it that clearly states what it does - be concise and clear.
- Every function must have an English comment above it that clearly states what it does - be concise and clear.
- Function, variable, file and package naming should follow recommended naming conventions for the project's specific language.


## Commands

- Build: `go build ./...`
- Run: `go run ./src/cmd/app`
- Test: `go test ./...`
- Format: `gofmt -l .` (fix with `gofmt -w .`)
- Vet: `go vet ./...`


## Project structure

```
src/             additional application packages must be created in their own folders under /src
  main.go        main package — binary entrypoint
  utilities/     packages that standardize contracts to external services
tests/           integration/end-to-end tests
.github/         workflows, issue templates, PR template
CHANGELOG.log
```

- Unit tests live next to the code they test (`foo_test.go` beside
  `foo.go`), per Go convention.
- `tests/` is reserved for integration/end-to-end tests that exercise the built binary together.
- New packages go under `src/internal/` unless they are meant to be
  imported by other modules, in which case use `src/pkg/`.

