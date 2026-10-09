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

## Config

vet reads `.vet.yaml` from the current directory if it exists. A file passed
with `--config` must exist. Unknown keys are an error.

```yaml
log_level: info   # debug | info | warn | error
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
`file` and `line` are omitted when they don't apply.
