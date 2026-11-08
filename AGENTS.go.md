# Go Project Conventions

Applies to all Go projects in addition to `AGENTS.universal.md`.

---

**CLI framework:** Use cobra. Register commands and flags in `init()` within
`cmd/` files. Use `RunE` (not `Run`) so errors propagate rather than being
handled inside the command.

**Doc comments:** Every exported symbol's doc comment must start with the
symbol's name (godoc convention): `// Server handles incoming HTTP requests.`

**Versioning:** Every binary declares a version variable stamped at build time:
```go
// Version is set at build time via ldflags.
var Version = "dev"
```
The Makefile stamps it via `-ldflags="-X <pkg>.Version=$(VERSION)"` using
`monova` for the version value.

**Error wrapping:** Always wrap with context — never return a bare error:
```go
return fmt.Errorf("load config: %w", err)
```

**Never ignore errors:** No `_ = fn()` or bare discards. If an error genuinely
can't be acted on, log it at trace level so `--trace` surfaces it:
```go
if err := f.Close(); err != nil {
    slog.Log(ctx, LevelTrace, "close file", "err", err)
}
```

**Logging:** Use `log/slog` with `slog.NewTextHandler`. Wire it from the
`--debug` / `--trace` flags as described in `AGENTS.universal.md`. Do not use
stdlib `log`.

**Server startup:** When the application starts a server on any port, log the
listening address unconditionally — to stderr and the trace log — regardless
of debug flags. Use a consistent format: `Listening on <addr>`. This ensures
operators and agents always know what address was bound.

**Testing:** Table-driven tests using `t.Run()` — one named sub-test per case.
Use only the standard `testing` package; no third-party assertion libraries.

**Build cache:** Use Go's default user-wide `GOCACHE` and `GOMODCACHE` so build
artifacts and downloaded modules are reused across repositories. Never set
either cache to a repository-local or project-specific directory in a Makefile
or development script. A temporary cache is acceptable when a restricted
sandbox cannot write to the default cache, but expect a cold rebuild and keep
the override scoped to that invocation. CI may use explicit persistent cache
directories managed by the runner.

Cache reuse requires identical build inputs. Projects that use the same native
stack should keep their Go toolchain, binding modules, `CGO_ENABLED`, compiler,
build tags, and CGO flags aligned where practical. Key CI caches by operating
system, architecture, Go version, and dependency lock state such as `go.sum`.
Do not run `go clean -cache` or build with `-a` routinely. Diagnose unexpected
rebuilds with `go env GOCACHE GOMODCACHE` before changing cache configuration.

**Commands & flags:**
- `--version` flag on root prints the version and exits. No short alias, no
  subcommand.
- `--debug` / `-d` and `--trace` are persistent flags on root so all
  subcommands inherit them without re-declaring.
- `--config` / `-c` sets the config file path. Default location:
  `~/.config/<app>/config.<ext>`.
- Only `--debug` (`-d`) and `--config` (`-c`) get short aliases. All other
  flags are long-form only.

**GTK applications:**
- When an application uses AI, expose model configuration in the UI as a
  priority-ordered list. Each model has its own provider, model name, endpoint,
  credentials, and **Test** action. Users can add, remove, and reorder models;
  show the effective order clearly and use that same order for failover.
- Make background work visible through one status and activity surface. Show
  the active operation with a spinner, optional progress, and its finished or
  failed state; identify the model that answered when AI is involved so
  fallback is visible. Put the status surface in an existing persistent bottom
  panel when the window has one. Otherwise, use a floating card in a
  `GtkOverlay` so appearing and disappearing does not reflow the main content.

**Directory layout (XDG):** Use `os.UserConfigDir`, `os.UserCacheDir`, and
`os.UserHomeDir` — never hardcode `~`. Respect the XDG env vars automatically
(`$XDG_CONFIG_HOME`, `$XDG_CACHE_HOME`, `$XDG_DATA_HOME`).

| Purpose | Default path |
|---------|-------------|
| Config file | `~/.config/<app>/config.<ext>` |
| Cache / downloaded data | `~/.cache/<app>/` |
| Persistent app state | `~/.local/share/<app>/` |
| Trace log | `/tmp/<app>.log` (truncated on start) |
