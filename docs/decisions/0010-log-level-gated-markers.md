# 0010: A pack may gate markers and facts by the engine's log level

**Date:** 2026-10-29 · **Decided by:** the owner · **Status:** decided, implemented 2026-10-29

llama.cpp at its default log level (verbosity 3) prints no build, no tensor load and no KV
cache line; at `-lv 4` it prints all three. A pack that declares markers for them could not
say "tested at `-lv 4`, not expected at the default level": `doriki pack test` fails a tested
marker that matches nothing, and a declared `kv.*` pair makes kv-alloc an expected phase, so
every default-level report would gain a `phase-unobserved` warning saying the engine may have
skipped it, which is false. This is the case decision 0003 named as its revisit trigger: a
marker the engine does not always log.

From now on, a pack may declare `detect.log_level_regex`, whose named group `level` reads the
engine's log level as a number from the first 200 log lines, and `min_log_level: N` on any
marker or fact. When the log's level is below N, or no line says the level, the entry is
dropped from the pack for that log: it is not matched, the phase it bounds is not expected,
and `pack test` lists it as skipped with the level it needs instead of failing it. A log with
no level line counts as the engine's default level. A pack that gates an entry without a
`log_level_regex`, or whose regex has no `level` group, is an error.

The rule lives in one place, `packs.AtLevel`, applied by `packs.Select` to the pack it returns
and by `doriki pack test`; `phase`, `rules` and the renderers are unchanged. Two alternatives
were weighed: a second pack file per log level, which duplicates every marker and needs a new
selection rule because both packs claim the same version, and marking the new entries
untested with confidence 0.8, which still adds the false warning and labels exact lines as
estimates.

Revisit if an engine names its levels (DEBUG, INFO) instead of numbering them, or prints no
level line, since the gate then cannot tell a verbose log from a default one.
