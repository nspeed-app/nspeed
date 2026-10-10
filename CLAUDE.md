# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`nspeed` is a network speed test / diagnostic CLI (module name `nspeed`, binary built from
`cmd/nspeed`). A single binary acts as both client and server, speaking HTTP/1.1, HTTP/2, HTTP/3
(and h2c), plus raw TCP/UDP/QUIC/QUIC-GO for throughput and latency measurement. It also embeds a
WASM web UI and JSON/HTML/text report generation.

This repo depends heavily on the sibling module `nspeed.app/nspeed` (imported as e.g.
`nspeed.app/nspeed/network`, `.../humanize`, `.../crypto`, `.../pacing`, `.../iobuffer`,
`.../generics`, `.../str`, `.../web`, `.../anonymize`). That module lives in a separate repo,
checked out as `../nspeed` (see `nspeed.code-workspace`, and the commented-out
`replace nspeed.app/nspeed => ../nspeed` in `go.mod`). Low-level networking/crypto/pacing
primitives generally belong there, not in this repo — if you're looking for e.g. QUIC dialing
helpers, IP-version handling, or byte-humanizing and can't find them here, check that repo.

## Commands

Build the CLI:
```
go build ./cmd/nspeed
```

Run it directly during development:
```
go run cmd/nspeed/main.go [global flags] <command> [command flags] ...
```
e.g. `go run cmd/nspeed/main.go -debug get -p 7777 -P quic https://target`. Use `-h` for global
flags and `<command> -h` for a command's own flags (see `nspeed/commands.go` for the list).

Run all tests:
```
go test ./...
```
Run a single test:
```
go test ./nspeed -run TestParseSchedules -v
```
Test files live under `nspeed/` (`schedule_test.go`, `message_test.go`, `client_pacing_test.go`,
`benchmark_test.go`).

Lint (same as CI, `.github/workflows/golangci-lint.yml`):
```
golangci-lint run
```
(`.golangci.yml` configures `errcheck`; `.revive.toml` configures the revive linter rules.)

Build the embedded WASM UI and launch it (via `magefile.go`, requires `mage`):
```
mage
```
This cross-compiles `nspeed/ui/goapp/main/main.go` to `nspeed/embed/ui/app.wasm` (GOOS=js
GOARCH=wasm), which `nspeed/ui.go` pulls in via `//go:embed embed/ui/*`, then runs
`go run cmd/nspeed/main.go ui`. You must rebuild the wasm blob after changing anything under
`nspeed/ui/goapp` or it won't be reflected when running/serving the UI.

Snapshot release build (goreleaser, see `.goreleaser.yml`):
```
./build-all.sh
```

## Architecture

### Command parsing and job scheduling

- `nspeed/commands.go` holds the `Commands` registry (`get`, `put`, `post`, `server`, `ciphers`,
  `api`, `bench`, `from`, `then`, `ui`). Each `Command` has a `Set` (commands in set `0` — e.g.
  `ciphers`, `ui` — must be used alone; others can be mixed within the same set) and a `FromAPI`
  flag gating whether the API endpoint (`nspeed/api.go`) is allowed to trigger it.
- `ParseCommands`/`ParseCommand` walk the CLI argv left-to-right, each command's `Parse` function
  (e.g. `CmdClientParseGet`, `CmdServerParse`, `CmdAPIParse`) consumes its own flags/args and adds
  `Job`s to a `JobList` (`nspeed/job.go`). `then` starts a new `Batch`; `from` reads a command
  script from a file/URL (templated with `text/template`, one command line per script line);
  `bench` expands a named benchmark template (`benchmark.go`) into one or more real commands and
  recursively calls `ParseCommands`.
- `nspeed.go`'s `RunJobs` is the actual scheduler: it runs batches sequentially. Within a batch,
  jobs are split into "infinite" (long-running: `server`, `api`, `ui`) and "short-lived"
  (`get`/`put`/etc.). Infinite jobs are started and awaited-ready first, then short-lived jobs run
  to completion; a batch ends when all short-lived jobs finish (unless it's the last batch with no
  short-lived jobs, in which case it runs forever until ctrl-C). Progress/results flow back through
  a `chan any` carrying `[]*metrics.Metric` or `*metrics.CPUMetric`.
- `Job` (`nspeed/job.go`) is the common interface: `Init`/`Run`/`IsInfinite` plus ID/GroupID/Batch
  bookkeeping via the embeddable `JobData`. Implementations: `JobClient` (client.go/http_client.go,
  the `get`/`put`/`post` commands), `JobHTTPServer` (server.go/http_server.go, the `server`
  command), `APIHost` (api.go, the `api` command, wraps a `JobHTTPServer` and mounts
  `/api/v1/...`), `UIHost` (ui.go, the `ui` command, also wraps a `JobHTTPServer`).

### Client/server transport

- `JobClient.Init` resolves the target URL's scheme and branches to `HTTPInit` (http/https —
  http_client.go, supports HTTP/1.1, HTTP/2, HTTP/3, h2c) or `L4Init`/`L4Run` (tcp/udp/quic/quic-go
  — client.go) which builds a raw `dialContext` per protocol (plain TCP, `golang.org/x/net/quic`,
  or `quic-go`) and speaks a small length-prefixed message protocol (`message.go`,
  `NewL4Message`/`WriteL4Message`) to tell the server GET or PUT.
- `HostParameters` (server.go) is the shared config struct for server/api/ui hosts (protocol, host,
  port, TLS settings, HTTP/1.1 / HTTP/2 / HTTP/3 toggles, max body size/duration, directory to
  serve, etc).
- `pacing.Schedule` (parsed by `ParseSchedules` in schedule.go from a `"wait,timeout,..."` string)
  drives paced/rate-limited reads and writes via `pacing.NewPacedReader`/`NewPacedWriter`.

### Metrics and reporting

- `metrics/metric.go` (`Metric`/`MetricsData`) implements `io.Reader`/`io.Writer` to
  transparently count bytes/time while a transfer runs, plus TLS/remote-addr metadata; `Metrics`
  aggregates all job results for a run.
- `metrics/textreport.go` / the JSON/HTML report helpers turn a `*metrics.Metrics` into the
  `-text`/`-json`/`-html` report outputs requested via CLI flags in `cmd/cli.go`.
- `metrics/cpu.go` / `cpu-otel.go` / `otel.go` sample host CPU (via `-cpu`/`-cpurate`) and can
  export through OpenTelemetry.

### Web UI and dashboards

- `nspeed/ui.go` serves the embedded WASM front-end (built with `maxence-charriere/go-app`, source
  in `nspeed/ui/goapp`) plus a websocket hub (`nspeed/ui/hub.go`, `nspeed/ui/client.go`) for
  pushing live data to connected browsers.
- `webview/` is a separate, simpler live-dashboard mechanism built on `go-echarts` (charts served
  via `webview/viewer.go`/`genchart.go`, JS glue in `webview/statics/nspeed-echarts.js`) plus
  OS-specific network interface counter collection (`net_linux.go`, `net_darwin.go`,
  `net_windows.go`, `net_fallback.go`) used for the CPU/network live viewer (`cpuviewer.go`,
  `netviewer.go`). `APIHost`/`UIHost` mount these routes when `-webstats`-style options are set.

### Entry point / global flags

- `cmd/nspeed/main.go` just calls `nspeed.Main()` in `cmd/cli.go`, which owns all global flags
  (logging verbosity, timeouts, IPv4/IPv6 forcing, report output paths, profiling, etc.),
  zerolog setup, and drives the run/report/shutdown sequence around `nspeed.RunJobs`.
- `cmd/json2html` is a small standalone helper binary for converting a saved JSON report to HTML.

### Other notes

- `_research/` is scratch/experimental code (protocol spikes, one-off tools) — not part of the
  shipped build, don't assume it compiles or is maintained.
- `dist/` is goreleaser build output; treat it as generated.
- `benchs/` holds predefined benchmark command scripts consumed by the `bench`/`from` commands
  (see `benchmark.go` for the template mechanism), not Go benchmark code.
