# CLAUDE.md

The estate's shared constitution (loaded by the `helios` plugin) is the authority. This
repository holds tools that serve every Helios repository; a change here reaches all of them.

- Go, standard library only unless a dependency is weighed and `govulncheck` is clean.
- `make check` is the one definition of green. Every command's logic that can be tested
  without the network is a pure function with a table-driven test; the `gh` calls are thin.
- `work` is what the unattended loop and the scheduler run; a behaviour change is announced in
  the engine's wiki `Runbook` before it merges.
- Pull requests to `main`, through the required checks.
