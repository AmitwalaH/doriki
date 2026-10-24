# How we work: read this first, every session

This file is how work gets done on this repo. Read all of it before doing anything, and follow
it even when a task looks small. If something here conflicts with what the owner says in the
chat, ask which one wins.

**We always work with three files:**

1. **This file** (`METHOD.md`): **how** we work. It rarely changes, and it is not specific to
   any one project.
2. **The project card** (`PROJECT.md`): **where** we are. The repo, the branch convention, the
   stack, the commands, what is frozen, what is off-limits. It changes when the project does.
3. **A task file** (`tasks/YYYY-MM-DD-<task>.md`): **what** we are doing now. The goal, the
   latest handoff, what is off-limits today, what "done" means.

Keep all three in mind for the whole session. **If the task file asks for something the method
forbids, the method wins.** If the project card and the method disagree on a detail of this
project, the card wins; if they disagree on a principle, the method wins and you ask.

---

## 1. The rules that never bend

1. **You never commit, push, merge, or open a pull request.** You change files and run checks,
   then you stop and report. A person reviews everything and does the version-control steps.
   **Use read-only version control only:** status, log, diff, show, grep. Never run anything
   that moves or discards work: checkout, switch, reset, restore, stash, clean, rebase, merge,
   pull, branch delete. **There may be uncommitted work you cannot see.**
   Exception: a project card may define a narrow, mechanical commit-and-push policy for task
   branches. If `PROJECT.md` has one, follow it exactly; if any condition is unclear, this rule
   applies as written. Pushing to the default branch, force-pushing, merging and opening pull
   requests are never delegated.
2. **You never edit a test or check that judges your work.** If one blocks you, stop and say so.
   **Never make a failing check pass by changing the check.** A judge you are allowed to edit is
   not a judge. This includes tests written by other people, and any check the project marks as
   supplied or owned elsewhere. Tests you write yourself under the method are yours to fix.
3. **You never hand-edit generated output.** If a file is produced by a build, a generator, a
   migration or a template, change the source and regenerate. Editing the output makes the
   generator and the file disagree, and the next build silently wins.
4. **You never touch anything the project card or the task file lists as off-limits.** Don't
   work around it either. Ask.
5. **If you cannot verify it, it does not ship.** Every change comes with a check that failed
   before and passes after, or a clear statement of why only a human can check it and exactly
   what they should look at.
6. **If you do not understand something, ask.** Don't guess what was meant, don't fill a gap
   with an assumption, and don't try something to see if it works. **A question costs a minute.
   A wrong guess on a shared branch costs much more.**

### How to ask

Ask as soon as you notice the gap, not after you have built on top of it. **Number every
question (Q1, Q2, ...)** so it can be answered in one line. For each one:

1. **What is unclear**, in one sentence.
2. **What you understood so far**, so it can be corrected.
3. **The options you see**, with your recommendation and why.
4. **What you will do meanwhile**, usually nothing, or only safe reading.

**Look facts up; ask only about choices.** Where a function lives, what a file does, which tests
exist, what the data says: find it yourself. Ask when it is a choice, or when the facts
contradict each other.

**Some choices are not yours.** Product behaviour, business rules, anything a recorded decision
covers, and anything that sets a precedent for work you cannot see. For those, write a
`[decision]` with options and a recommendation, say so, and either wait or move to parts of the
task that do not depend on it.

**Ask when any of these happen:**

- The task, ticket or handoff can be read two ways.
- The code does something on purpose that the task calls a bug. A recorded decision, a comment
  explaining why, or a test that pins it are all signs of on purpose.
- The fix would touch more files than the task approved, or anything off-limits.
- A check fails and you do not know why.
- What you see does not match what the task file says.
- **You are about to write "probably" about something that matters.**
- An instruction blocks something the task needs. Ask whether a narrower action is acceptable
  and who should do the blocked part.

**It is always fine to say "I do not know yet." It is never fine to sound sure when you are
not.**

---

## 2. Start of every session

In order, before any code:

1. **Read the task file.** It holds the latest handoff: what landed, which decisions were
   answered, what someone else is in the middle of, what is wanted next. If anything there is
   unclear or looks out of date, **ask before starting**.
2. **Read the project card**, and the referenced docs and open tickets that touch this task.
3. **Check the branch state** with read-only commands. Say so if it is the wrong branch, or if
   anything is uncommitted or unexpected. **Don't clean it up yourself.**
4. **Record the baseline.** Run the project's full check suite **before you touch anything** and
   write down what is already failing. **A red you did not cause is not yours to fix, and a red
   you cannot tell apart from your own is worse than a red you can.**
5. **Say which files you expect to touch**, so collisions can be caught early.

---

## 3. The method, step by step

Work through these in order. **Don't skip one because the fix looks obvious.** Most of the best
fixes came from a step that looked unnecessary.

**You stop and wait at three checkpoints:**

1. **After Step 4**, the plan. Report steps 0 to 4 and wait. No code, no test files, no edits.
2. **Whenever rule 6 applies.**
3. **At Step 12**, the report.

Between checkpoints, keep going on your own. **Don't stop for things you can look up.**

**Bug or new work?** Steps 1 and 2 are written for bugs. For new work, Step 1 becomes "describe
the current state, from evidence", and Step 2 becomes "show exactly where the new piece plugs
in". Every other step is the same.

### Step 0: Is it already known?

Search the tickets, the plans and recent commits. **If a ticket exists, read it.** It may record
a decision that must be respected, or an earlier measurement that is now stale. **A recorded
decision is not overturned without asking.**

### Step 1: Reproduce it

Say exactly what a user sees, step by step. Reproduce it from **evidence**: a failing run, the
data, a small script, or walk notes someone else took. **If you cannot open the thing yourself,
say so and give exact steps for a person to check.** If the evidence does not show the problem,
say that and do not fix it.

### Step 2: Find the cause

Find the exact lines and explain why they produce what is observed. **Cite file and line.**
Label guesses as guesses.

### Step 3: Search before you write

Before writing anything new, look for code, helpers, types or patterns that already do it.
**Reuse the code, not the idea.** Then ask who else uses what you are about to change. That is
the blast radius, and it belongs in the report.

**Look outside too, briefly.** If this is a problem other people have, find out how they solved
it before inventing a solution. **Say what you found and whether it applies.** "I looked and
there is no standard approach" is a useful finding.

### Step 4: Choose the solution, never the first one that works

Write two or three real options in a table. **No straw men.** Judge each on four things:

| Criterion             | The question                                                                                                                 |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| **Scalable**          | Does it still work on the next case? Nothing about one instance hardcoded.                                                   |
| **Maintainable**      | **One encoding per fact.** One place for a key, a list, a rule, a threshold. Reuses existing code. A short comment says why. |
| **Manageable**        | The smallest change that is right. One decision per change. Nothing frozen moved, no judge broken.                           |
| **Best for the user** | Honest in what it shows, respects recorded decisions, fixes the cause rather than the symptom.                               |

Then recommend one, say **what it depends on**, and name a **revisit trigger**: the thing that
would make this choice wrong later.

**If it is a product or business choice, it goes to the owner** as a `[decision]` with your
recommendation. **Don't decide it silently.**

**Checkpoint 1: stop here.**

### Step 5: No over-engineering

Applies to the options you propose, not just the code.

- Solve today's problem, not imagined future ones.
- **No new abstraction, framework, config layer or file unless the task needs it.**
- Prefer editing one file to creating three.
- If a bigger, cleaner refactor is tempting, **write it as a ticket instead of doing it.**
- **A change touching three or more files is raised before you go past two**, unless the task
  file approves a file list. Then stay inside that list.

### Step 6: Define the observable change

In one or two sentences: **what will someone see after this that they do not see now?** This is
what gets checked.

### Step 7: Write the test first, and watch it fail

- Write it **before** the fix. Run it. **Show that it fails for the right reason.**
- The test says what the code **should** do, not what it currently does.
- **Check the real output, never the source text.** Render it and read it. Run it and check the
  values. **Never test by searching the source for a word.** A search can prove something was
  removed; it cannot prove the right thing arrived, and it passes deliberately wrong fixes.
- **Ask: would a plausible wrong fix also pass this test?** Name one and say why it would not.
- **Where a rule is meant to be shared, prove the sharing.** Replace the shared rule with
  sentinel values and assert every caller changes. **A test using the real values passes a fix
  that sets each caller's own copy to the same number**, which is the thing you were trying to
  stop.
- If the only honest check is a human looking at it, **say so.** Don't write a test that can
  never fail.

### Step 8: Make the fix

Only the change chosen in Step 4. Follow the file's existing style. **Comments explain why, in
plain words.**

### Step 9: Ask "where else?"

The bug came from an assumption. **Search for the same assumption elsewhere.** Fix only what
today's task needs and list the rest as tickets. **The class is usually worth more than the
instance.**

### Step 10: Predict, then run the checks

**Write down what you expect each check to say before you run it.** A prediction that comes true
is evidence; a prediction that fails is a finding. **Both are worth more than a result with no
prediction in front of it.**

Then run everything the project card lists, and **paste the results**.

What green means:

- **No new type errors.** The baseline may go down, never up.
- **No new lint errors.** Pre-existing warnings in a file you did not cause are fine, say so.
- **Zero failing tests**, other than the ones the project card names as failing by design.
- **Every red accounted for**, by name, as yours or not yours.

### Step 11: Frozen and pinned things

If a snapshot, hash, baseline or golden file fails:

- **Stop and say so before re-recording anything.** Say which value failed, which of your files
  caused it, and whether the user-visible behaviour changed.
- **If it should not have changed, find out why first.**
- Once approved, re-record **one value at a time**, running the check between each.
- **Re-record once per change.** If two pieces of work both re-record it, they belong together.
- **When two people change it**, whoever lands second recomputes over the merged result.
  **Neither side's old value is correct after two changes.**
- The re-record goes in **its own commit on an otherwise unmodified tree.**
- **Add a dated note** saying what changed and why the behaviour did or did not move.

### Step 12: Report and stop

Don't commit. Give the report in section 6 and wait.

---

## 4. Judgment and verification

**This is the most important part.** Tools catch type errors. People catch "this would
embarrass us in front of a customer".

- **Judgment:** weigh every finding by what a user would see. **Something false on screen, the
  wrong customer's data, lost work, a crash: blockers.** A heading in the wrong words: polish.
- **Verification:** nothing is done until it is checked against the test, the checks, and the
  real thing. **Say exactly what a person should look at.**
- **"I think" and "I checked" are different words. Use the right one.**
- **Don't overclaim.** If you checked one case, do not write "all cases".
- **Expect to be asked "are you sure?"** Claims get checked against this file. **That is normal,
  not mistrust.** The right answer is often "no, here is what I actually verified".
- **Measure, do not inherit.** A number in a document was true when it was written. **Every
  count has a shelf life.** Re-measure anything you are about to rely on, and say when you
  measured it.
- **Instruments lie.** A regex with a missed case, a command that silently skips files, a test
  config that finds nothing and reports a pass. **When a tool gives you a confident answer,
  ask what it would have done if the answer were different.**
- **Name the falsifier before the work.** Say what result would prove you wrong, then look for
  it. **A finding you predicted and then confirmed is worth more than one you found afterwards.**
- **For anything high-stakes, use two independent readers.** State the answer yourself with a
  source for every value, then have the same question answered independently, and compare value
  by value. **A disagreement is a finding, not noise.**
- **Check meaning, not just shape.** The right names with the wrong numbers is still wrong.

---

## 5. Where findings go

**Every finding has exactly one home.** A finding living in a chat message, a pull request body
or a commit message is a finding that will be lost.

| What you found                                                    | Where it goes                                     | Label            |
| ----------------------------------------------------------------- | ------------------------------------------------- | ---------------- |
| A new bug, not fixed in this task                                 | A ticket                                          | `[ticket]`       |
| A product or business question                                    | Its own ticket, with options and a recommendation | `[decision]`     |
| Something that must be fixed before this change lands             | The change description                            | `[blocks merge]` |
| An effect on something outside this task, or a deliberate non-fix | The change description                            | none             |

**Ticket file name:** `{PREFIX}-{CAT}-{BUG|ENH}-{YYYYMMDD}-{short-slug}.md`, with the prefix and
categories set in the project card.

**Ticket template:**

```
# {ID}: {One-line title in user-visible terms}

**Date Discovered:** YYYY-MM-DD
**Severity:** HIGH | MEDIUM | LOW
**Status:** OPEN
**Label:** [ticket] | [decision] | [blocks merge]
**Found by:** {who}, {how: walked, measured, read in code}
**Filed by:** {who}

## Problem
What someone sees, step by step. Then the cause, with file and line.

## Why it matters
What it costs, and how a user would read it.

## Options (for decisions)
Two or three, with trade-offs and a recommendation.

## Related
Other tickets, changes, plans.
```

**Write tickets in user-visible terms first**, and **say how you know**: walked, measured by
running the code, or read in the source. Those are three different strengths of evidence and
the reader needs to know which one they have.

---

## 6. The report at the end of every task

Since you do not commit, give this, in order:

1. **Summary** in user-visible terms: before and after.
2. **Files changed**, and why each one.
3. **The options weighed** and why this one.
4. **Checks:** each command, what you predicted, what you got.
5. **Frozen things:** passed, or what was re-recorded and why.
6. **What a person should check**, with exact steps.
7. **"Where else" findings** and the tickets drafted.
8. **Open questions**, numbered.
9. **A proposed commit message.**
10. **A draft change description.**

**Say what you could not verify.** On any report that informs a decision, that section is worth
more than the rest, because it is the list of things someone still has to look at.

---

## 7. Change descriptions

**Keep each change to one decision.** Research on review quality is consistent: defect detection
falls sharply once a reviewer is past a few hundred lines. **A change too large to review in an
hour is too large, not owed more time.**

```
{One line: what was wrong, in user-visible terms.}

What someone sees:
- Before: ...
- After: ...

What else this touches: {other cases, other users, anything frozen}

Chosen on purpose: {deliberate non-fixes}

Checks: {suite, types, lint, frozen things}

Filed:
- [ticket] ...
- [decision] ... Recommendation: ...
```

---

## 8. Spikes

**A spike is an hour of work to answer one question, and it is thrown away.**

Use one when a plan rests on a claim nobody has tested. **Write the question, the prediction and
the falsifier before you start.** Keep the code in its own directory, imported by nothing. **A
spike that kills the plan is a success**, and cheaper than finding out during the build.

**The report is the deliverable, not the code.**

---

## 9. End-of-day handoff

```
Done: {what landed, one line each}
Open: {what is in progress, and where}
Need from you: {decisions, each with my recommendation}
Files I am in: {so nobody collides}
Measured at: {the commit, and when}
```

**A handoff is a dated measurement, not a standing truth.** When you read one at the start of a
session, **check its claims before building on them.** When you write one, **say when and
against what you measured**, so the next reader knows what has a shelf life.

---

## 10. Practical notes

- **Test on a clean state.** Fresh window, fresh session, cleared storage. **Stale local state
  shows you yesterday's answer.**
- **Scratch files go outside the repo.** Say where, and delete them when done.
- **Never put secrets in code, tests, docs or logs.**
- **Never put a real person's name in code or comments.**
- **Writing style:** plain, short sentences. No em dashes.
- **When in doubt, stop and ask.**
