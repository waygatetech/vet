# vet

`vet` runs the agent and deterministic checks that gate plans and diffs.
It knows nothing about ticket state: tix hooks call it and record its output.

> **Not `go vet`.** This is a standalone binary that happens to share the word.
> It does not wrap or replace the Go toolchain's `go vet`.

vet is the companion to [tix](https://github.com/waygatetech/tix), a ticket
tracker for agent-driven work. In tix, each ticket gets a plan file
(`plans/<ticket>.md`), and hooks run at plan and done time. vet is what those
hooks call. Its [exit codes](#exit-codes) tell the hook whether to pass or
block. vet also runs on its own: any script or CI step can call it and use the
exit code.

## Requirements

- `git`: `review` and `lock` read the diff and the merge base from git.
- An agent CLI that reads a prompt on stdin and prints its answer. The default
  is `claude -p` ([Claude Code](https://claude.com/claude-code)). Use any other
  CLI by setting `critic_command` and `reviewers` in [`.vet.yaml`](#config).

## Install

```sh
go install github.com/waygatetech/vet@latest
```

Or download a binary from [Releases](https://github.com/waygatetech/vet/releases).

## Example

A plan is Markdown with YAML frontmatter:

```markdown
---
ticket: x-1
tests: ["internal/foo/foo_test.go"]
contracts_changed: []
---
# Add Foo

Acceptance: `Foo("")` returns an error.
```

Then a ticket goes through these steps:

```sh
vet critique plans/x-1.md --ticket ticket.txt  # writes plans/x-1.critique.md
# answer each C<n> under critique_responses in the plan, then:
vet critique --check plans/x-1.md

# a separate agent writes internal/foo/foo_test.go, then:
vet lock --plan plans/x-1.md
git add internal/foo/foo_test.go .vet/locks/x-1.json && git commit -m "Lock x-1 tests"

# implement, then:
vet review --plan plans/x-1.md
vet lock --check --plan plans/x-1.md
```

## Plan file

vet reads these fields from a plan's YAML frontmatter and ignores any others
(tix reads its own). A plan with frontmatter that isn't closed with `---`, or
that isn't valid YAML, is an error for every command that reads it.

| Field                | Type                    | Read by                                    | Required                                                         |
| -------------------- | ----------------------- | ------------------------------------------ | ---------------------------------------------------------------- |
| `ticket`             | string                  | `vet lock`, `vet lock --check`             | Yes, for `vet lock`. A plain file name: no `/`, not `..`.        |
| `tests`              | list of globs           | `vet lock`, `vet lock --check`             | No. Without it, `--check` passes. `**` spans directories.        |
| `critique_responses` | map of `C<n>` to string | `vet critique --check`                     | One non-empty answer per item in `plans/<plan>.critique.md`.     |
| `contracts_changed`  | list of strings         | `vet review` (`contracts_changed` trigger) | No. Any entry triggers reviewers with `contracts_changed: true`. |

## Usage

```sh
vet --version
vet --config path/to/.vet.yaml ...
```

### critique

```sh
vet critique plans/x-1.md --ticket ticket.txt [--context why.txt ...]
vet critique --check plans/x-1.md
```

`vet critique` gives a fresh-context critic only the ticket text, the plan,
`./concepts.yaml`, and any `--context` files. It never sees the planning
transcript. The critic looks for failure modes, missing or untestable acceptance
criteria, overlap with existing concepts, and undeclared contract changes.
vet writes the result to `plans/x-1.critique.md`, with one `## C<n>` item per
finding. If the critic fails or returns malformed output, vet exits 3.

`--check` exits 1 unless every item has a non-empty answer in the plan's
frontmatter:

```yaml
critique_responses:
  C1: "Added acceptance test for empty input."
  C2: "Accepted risk: single writer only."
```

### review

```sh
vet review --plan plans/x-1.md [--base main] [--tier 2] [--context why.txt ...] [--json]
```

`vet review` gives a fresh-context reviewer only the plan, the diff, and any
`--context` files (for example human rulings, so it can flag changes that
contradict them without a supersedes entry). It never sees the implementation
transcript. The diff runs from the merge base of `--base` and `HEAD` to the
working tree, plus untracked files. vet prints the findings, or the JSON report
with `--json`, and exits 1 on any blocking finding. If the reviewer fails or
returns malformed output, vet exits 3.

Every reviewer in `reviewers` whose trigger matches runs concurrently with the
same input. Each finding carries the `reviewer` that raised it. A block from
any reviewer fails the gate, and so does a failure from any reviewer. If no
reviewer triggers, vet exits 2. `--tier` passes the ticket's review tier (from
tix) for `min_tier` triggers. It defaults to 0.

### lock

```sh
vet lock --plan plans/x-1.md
vet lock --check --plan plans/x-1.md
```

`vet lock` freezes a plan's acceptance tests so the implementer can't redefine
done by changing them. It's opt-in: set `test_command` in `.vet.yaml`. The plan
names its tests in frontmatter with globs, where `**` spans directories:

```yaml
tests: ["internal/foo/foo_test.go"]
```

Once a separate agent has written the tests, `vet lock` hashes every tracked or
untracked (not ignored) file that matches. It writes the hashes, the globs, and
`HEAD` to `.vet/locks/<ticket>.json`. Commit the tests and the lock before
implementing. It refuses to overwrite an existing lock unless the plan's
`tests` globs changed. Unlocking therefore means amending the plan, which the
plan hook re-reviews.

`--check [--base main]` (run it from the tix done hook) ignores the
working-tree lock and compares against the one committed at the merge base of
`--base` and `HEAD`, so deleting or rewriting the lock doesn't help. The
exception is a plan whose `tests` globs differ from the committed lock's (an
amendment): then it uses the re-lock. It exits 1 when:

- the plan has tests but no lock is committed at the merge base;
- a matching file was changed, added, or removed;
- the plan's globs differ from the locked ones;
- `test_command` fails.

A plan without `tests` passes.

## Config

vet reads `.vet.yaml` from the current directory if it exists. A file passed
with `--config` must exist. Unknown keys are an error.

```yaml
log_level: info                 # debug | info | warn | error
critic_command: [claude, -p]    # argv for vet critique; prompt is sent on stdin
reviewers:                      # agents for vet review; prompt is sent on stdin
  - name: claude                # default when reviewers is empty
    command: [claude, -p]
  - name: codex                 # cross-model second opinion
    command: [codex, exec]
    when:                       # omit to always run; any one condition triggers
      contracts_changed: true   # plan lists contracts_changed
      min_tier: 2               # vet review --tier >= 2
      paths: ["migrations/**"]  # diff touches a match; ** spans directories
test_command: [go, test, ./...] # argv for vet lock --check; unset disables vet lock
```

## Exit codes

| Code | Meaning                                         |
|------|-------------------------------------------------|
| 0    | Pass: no blocking findings                      |
| 1    | Blocking findings                               |
| 2    | Usage or config error                           |
| 3    | Reviewer/agent failure (the gate fails closed)  |

## Findings

Checks emit a JSON report:

```json
{
  "findings": [
    {"check": "paths", "severity": "blocking", "file": "main.go", "line": 12, "message": "..."}
  ]
}
```

`severity` is one of `blocking`, `warning`, or `info`. Only `blocking` fails the gate.
`file`, `line`, and `acceptance_ref` (the acceptance criterion a review finding
relates to) are omitted when they don't apply.
