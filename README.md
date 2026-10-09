# vet

`vet` runs the agent and deterministic checks that gate plans and diffs.
It knows nothing about ticket state: tix hooks call it and record its output.

> **Not `go vet`.** This is a standalone binary that happens to share the word.
> It does not wrap or replace the Go toolchain's `go vet`.

## Install

```sh
go install github.com/waygatetech/vet@latest
```

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
