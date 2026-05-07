# Cobra Notes

Notes on working with the Cobra CLI framework in this codebase — flag access patterns,
persistent-flag scoping, and precedence helpers.

## Cobra Flag Access Notes

- `cmd.Flags().GetString("foo")` only sees LOCAL flags until `mergePersistentFlags()` runs (during `Execute` or `ParseFlags`). Calling it from a unit test that drives a function directly (without Execute) will fail with `flag accessed but not defined`. Use `cmd.Flag("foo").Value.String()` instead — `Flag()` falls through to persistent flags + parents' persistent flags via `persistentFlag()`/`updateParentsPflags()`. Same applies to GetBool — for bool defaults that overlap with "unset" (e.g. `--insecure` defaults false), gate on `cmd.Flag("name").Changed` to disambiguate.
- For first-non-empty-wins precedence pass values to a helper that picks the FIRST non-empty. For lowest→highest precedence (`.env` < env < flag) use a `last-non-empty-wins` helper instead — easy to swap accidentally; query/connect.go calls this `overlay()`.
