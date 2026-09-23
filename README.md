# HeliosTools

Tools for working on the Helios estate.

## work

The token-cheap view of GitHub Issues. Agents never page the API: one call mirrors a
repository's open issues into a gitignored `issues.jsonl`, selection reads the mirror, and the
few writes go through `gh`. Run it inside a repository checkout; `gh` infers the repository.

```
go install github.com/heliosbbs/heliostools/cmd/work@main

work refresh                       mirror this repository's open issues
work next [--loop]                 the one issue to work on, as JSON (exit 3 if none)
work claim <n>                     label it claimed, comment "Claimed by <identity>"
work tick <n> <task> <comment>     tick plan box <task>, post the comment
work close <n> <pr> [writeup-file] close when every box is ticked and the PR is merged
work lint                          every open issue's shape; exit 1 on any fault
work unclaim-stale <days>          release claims with no activity for <days>
work stale [--apply] [--ref <r>] <path>...
                                   open plans whose unticked tasks cite a changed
                                   document; --apply labels them blocked, comments once
```

Selection is mechanical: the caller's own claimed item, else the first open item by priority
(`P0` to `P3`, unlabelled last) then age, skipping `blocked`, `human-action-required`, anything
with an open dependency, and other identities' claims; with `--loop`, only items labelled
`unattended-loop`. The identity is `WORK_IDENTITY` or `git config user.name`.

A plan box is a `- [ ]` line under the issue's `### Plan` heading; a struck task is ticked with
its reason in the text (`- [x] ~~3. thing~~ struck: reason`).
