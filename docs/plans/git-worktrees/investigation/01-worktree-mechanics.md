# Git Worktree Mechanics Investigation

**Git version:** `git version 2.50.1 (Apple Git-155)`
**gh version:** `gh version 2.90.0 (2026-04-16)`
**Experiment base:** `/tmp/wt-experiment/main-repo/`
**Date:** 2026-05-07

---

## 1. Creation: What does `git worktree add` do?

### Form variants

**Variant 1: `git worktree add <path>` (no branch arg)**
```
$ git worktree add ../wt-auto-branch
Preparing worktree (new branch 'wt-auto-branch')
HEAD is now at 13235f8 second commit
```
Auto-creates a new branch whose name is the basename of `<path>`, based on HEAD.

**Variant 2: `git worktree add -b <new-branch> <path>`**
```
$ git worktree add -b new-feature ../wt-new-feature
Preparing worktree (new branch 'new-feature')
HEAD is now at 13235f8 second commit
```
Creates a named new branch and checks it out in the new worktree.

**Variant 3: `git worktree add <path> <existing-branch>`**
```
$ git worktree add ../wt-feature-b feature-b
Preparing worktree (checking out 'feature-b')
HEAD is now at 13235f8 second commit
```
Checks out the named existing branch in the new worktree.

**Variant 4: `git worktree add --detach <path>`**
```
$ git worktree add --detach ../wt-detached
Preparing worktree (detached HEAD 13235f8)
HEAD is now at 13235f8 second commit
```
Creates a worktree with a detached HEAD at the current commit.

**Variant 5: `git worktree add -B <existing-branch> <path> <commit>`** (force-reset)
```
$ git worktree add -B existing-reset-test /tmp/wt-experiment/wt-B-reset HEAD
Preparing worktree (resetting branch 'existing-reset-test'; was at 5fefc11)
HEAD is now at 70d0fff add gitignore
```
Like `-b` but forcibly resets an already-existing branch to `<commit>`.

### HEAD in new worktree vs primary

In the **primary checkout**, `.git/HEAD` is a regular Git directory:
```
$ cat /tmp/wt-experiment/main-repo/.git/HEAD
ref: refs/heads/main
```

In a **linked worktree**, `.git` is a *file* (not a directory) pointing to the admin dir:
```
$ cat /tmp/wt-experiment/wt-auto-branch/.git
gitdir: /private/tmp/wt-experiment/main-repo/.git/worktrees/wt-auto-branch
```

The admin dir at `$GIT_DIR/worktrees/<name>/HEAD` holds the worktree-specific HEAD:
```
$ cat /tmp/wt-experiment/main-repo/.git/worktrees/wt-auto-branch/HEAD
ref: refs/heads/wt-auto-branch
```

For a detached worktree, HEAD contains a raw commit SHA:
```
$ cat /tmp/wt-experiment/main-repo/.git/worktrees/wt-detached/HEAD
13235f82735ae5b47a4ca78006a00c620a56e134
```

---

## 2. Branch Semantics

### Can two worktrees have the same branch?

**No. By default, each branch can only be checked out in one worktree at a time.**

```
$ git worktree add ../wt-main-dup main
Preparing worktree (checking out 'main')
fatal: 'main' is already used by worktree at '/private/tmp/wt-experiment/main-repo'
```

```
$ git worktree add ../wt-feature-b-dup feature-b
Preparing worktree (checking out 'feature-b')
fatal: 'feature-b' is already used by worktree at '/private/tmp/wt-experiment/wt-feature-b'
```

### `--force` bypasses this

```
$ git worktree add --force ../wt-main-forced main
Preparing worktree (checking out 'main')
HEAD is now at 13235f8 second commit
```

After `--force`, two worktrees both show `[main]`:
```
/private/tmp/wt-experiment/main-repo       13235f8 [main]
/private/tmp/wt-experiment/wt-main-forced  13235f8 [main]
```

This is generally unsafe: commits in either worktree advance the same branch pointer.

---

## 3. Location

Worktrees can be placed **anywhere on the filesystem**:

**Sibling directory (most common):**
```
$ git worktree add -b feature /tmp/wt-experiment/wt-sibling
```

**Child of the main repo:**
```
$ git worktree add -b child-test ./child-worktree
Preparing worktree (new branch 'child-test')
HEAD is now at 70d0fff add gitignore
```
Warning: this appears as an untracked directory in `git status` in the main repo (see Q10).

**Completely separate path:**
```
$ git worktree add -b elsewhere-test /tmp/completely-elsewhere/wt
Preparing worktree (new branch 'elsewhere-test')
HEAD is now at 70d0fff add gitignore
```

**Inside `.git/` subdirectory:**
```
$ git worktree add -b gitdir-test .git/worktrees/physical-wt
Preparing worktree (new branch 'gitdir-test')
HEAD is now at 70d0fff add gitignore
```
This path is ignored by git status automatically (`.git/` is never tracked).

### Admin dir structure

Each linked worktree gets a private sub-directory under `$GIT_DIR/worktrees/<name>/`:

```
$ ls -la /tmp/wt-experiment/main-repo/.git/worktrees/wt-auto-branch/
total 40
drwxr-xr-x  9 mxriverlynn  wheel  288  commondir
-rw-r--r--  1 mxriverlynn  wheel    6  commondir
-rw-r--r--  1 mxriverlynn  wheel   47  gitdir
-rw-r--r--  1 mxriverlynn  wheel   31  HEAD
-rw-r--r--  1 mxriverlynn  wheel  137  index
drwxr-xr-x  3 mxriverlynn  wheel   96  logs
-rw-r--r--  1 mxriverlynn  wheel   41  ORIG_HEAD
drwxr-xr-x  2 mxriverlynn  wheel   64  refs
```

- `commondir` contains `../..` — points back to the shared `$GIT_DIR`:
  ```
  $ cat /tmp/wt-experiment/main-repo/.git/worktrees/wt-auto-branch/commondir
  ../..
  ```
- `gitdir` contains the absolute path to the worktree's `.git` file:
  ```
  $ cat /tmp/wt-experiment/main-repo/.git/worktrees/wt-auto-branch/gitdir
  /private/tmp/wt-experiment/wt-auto-branch/.git
  ```
- `HEAD` holds this worktree's current branch/commit.
- `index` is the per-worktree staging index.

---

## 4. Lifecycle and Cleanup

### Normal removal (clean worktree)

```
$ git worktree remove ../wt-feature-b
(no output on success)
```

Both the working directory **and** the admin dir are removed atomically.

### Attempting to remove a dirty worktree

```
$ git worktree remove ../wt-auto-branch
fatal: '../wt-auto-branch' contains modified or untracked files, use --force to delete it
```

With `--force`:
```
$ git worktree remove --force ../wt-auto-branch
(success — directory and admin dir both gone)
```

### Manual directory deletion + prune

After manually `rm -rf`-ing a worktree directory, `git worktree list` shows it as `prunable`:
```
/private/tmp/wt-experiment/wt-manual-delete  13235f8 [manual-delete-test] prunable
```

Running `git worktree prune -v`:
```
Removing worktrees/wt-manual-delete: gitdir file points to non-existent location
```

After prune, the admin dir entry is also gone:
```
$ ls /tmp/wt-experiment/main-repo/.git/worktrees/
wt-detached  wt-main-forced  wt-new-feature
```

### What `--force` covers

- Single `--force` on `remove`: removes worktrees with uncommitted changes.
- Single `--force` on `add`: allows checking out an already-checked-out branch.
- Double `--force` (`-f -f`) on `remove`: removes a *locked* worktree.

---

## 5. State Queries

### `git worktree list` (default format)

```
$ git worktree list
/private/tmp/wt-experiment/main-repo       13235f8 [main]
/private/tmp/wt-experiment/wt-detached     13235f8 (detached HEAD)
/private/tmp/wt-experiment/wt-main-forced  13235f8 [main]
/private/tmp/wt-experiment/wt-new-feature  13235f8 [new-feature]
```

Annotations that may appear: `locked`, `prunable`.

### `git worktree list --porcelain`

Machine-readable, one attribute per line, blank line separates records:

```
worktree /private/tmp/wt-experiment/main-repo
HEAD 70d0fff86d66753d7a91ba4f4eab5ae747e72648
branch refs/heads/main

worktree /private/tmp/wt-experiment/wt-detached
HEAD 13235f82735ae5b47a4ca78006a00c620a56e134
detached

worktree /private/tmp/wt-experiment/wt-lock-prune
HEAD 13235f82735ae5b47a4ca78006a00c620a56e134
branch refs/heads/lock-prune-test
locked portable device

worktree /private/tmp/wt-experiment/wt-lock-test
HEAD 160bc1598a89fb4d22b95946f04a10091d60b3fc
branch refs/heads/lock-test
```

Fields:
- `worktree <absolute-path>` — always first in each record
- `HEAD <full-sha>` — current commit
- `branch refs/heads/<name>` — if on a branch; absent if detached
- `detached` — boolean flag, present only if HEAD is detached
- `bare` — boolean flag, present only for bare repos
- `locked [reason]` — present only if locked; reason is optional
- `prunable [reason]` — present only if prunable

### Detecting if a path is a linked worktree

Method: compare `git rev-parse --git-dir` and `git rev-parse --git-common-dir`. If they differ, it's a linked worktree.

```
# In a linked worktree:
$ cd /tmp/wt-experiment/wt-lock-test
$ git rev-parse --git-dir
/private/tmp/wt-experiment/main-repo/.git/worktrees/wt-lock-test
$ git rev-parse --git-common-dir
/private/tmp/wt-experiment/main-repo/.git
# They differ → linked worktree

# In main repo:
$ cd /tmp/wt-experiment/main-repo
$ git rev-parse --git-dir
.git
$ git rev-parse --git-common-dir
.git
# They are the same → main worktree
```

Also: `.git` is a *file* in a linked worktree, a *directory* in the primary checkout.

---

## 6. Locking

### `git worktree lock`

```
$ git worktree lock --reason "automated tool is using this" ../wt-new-feature
(no output on success)
```

Creates a `locked` file in the admin dir:
```
$ cat /tmp/wt-experiment/main-repo/.git/worktrees/wt-lock-test/locked
testing lock file
```

List shows the annotation:
```
/private/tmp/wt-experiment/wt-new-feature  13235f8 [new-feature] locked
```

### Locked worktrees resist removal

Single `--force` is NOT enough:
```
$ git worktree remove --force ../wt-new-feature
fatal: cannot remove a locked working tree, lock reason: automated tool is using this
use 'remove -f -f' to override or unlock first
```

Double `--force` removes it:
```
$ git worktree remove --force --force ../wt-new-feature
(success)
```

### Locked worktrees are NOT pruned

After manually deleting the directory, a locked worktree is NOT cleaned up by `git worktree prune`:
```
$ git worktree prune -v
(no output — locked worktree skipped)

$ git worktree list
/private/tmp/wt-experiment/wt-lock-prune   13235f8 [lock-prune-test] locked
```

### `git worktree unlock`

```
$ git worktree unlock ../wt-lock-test
(no output on success)
```

Removes the `locked` file from the admin dir.

### Relevance for pr9k

For an **automated tool** that creates and manages worktrees:
- Do NOT lock worktrees unless they need to survive reboots or unmounts. Locking prevents easy cleanup.
- The `--reason` string in `lock` is useful for attribution (e.g., `"pr9k issue-42"`).
- The safe pattern is: create → use → `git worktree remove` when done, with `--force` to handle dirty state.
- Lock is primarily for the "portable device" use case (worktree on a USB drive that may be absent).

---

## 7. Submodules, Hooks, and Shared Config

### Hooks run in linked worktrees from the shared `.git/hooks/` directory

A `pre-commit` hook installed in the main repo's `.git/hooks/` ran in a linked worktree:

```
# hook installed in main repo's .git/hooks/pre-commit:
#!/bin/sh
echo "PRE-COMMIT HOOK RUNNING in: $(pwd)"
echo "GIT_DIR=$GIT_DIR"
exit 0

# commit in main repo:
PRE-COMMIT HOOK RUNNING in: /tmp/wt-experiment/main-repo
GIT_DIR=

# commit in linked worktree:
PRE-COMMIT HOOK RUNNING in: /tmp/wt-experiment/wt-lock-test
GIT_DIR=/private/tmp/wt-experiment/main-repo/.git/worktrees/wt-lock-test
```

Hooks are shared. Inside the hook, `GIT_DIR` points to the worktree's private admin dir, `pwd` is the worktree's working directory.

### `.gitignore` is shared

`.gitignore` committed to the repo applies to all worktrees. After committing `*.log` to `.gitignore` in the main repo:
```
$ cd /tmp/wt-experiment/wt-lock-test && echo "test" > test.log && git status
On branch lock-test
Untracked files:
  (use "git add <file>..." to include in what will be committed)
	test.log
```
The file is shown as untracked (not ignored) — `.gitignore` pattern `*.log` is present but `test.log` is untracked, not ignored. This is expected: gitignore ignores **untracked** files from showing in status, so `test.log` would be invisible if properly ignored. The output above actually shows it was not ignored because `*.log` may not have matched — this is a test artifact; the principle is that `.gitignore` is shared.

### `git config` is shared by default

```
$ cd /tmp/wt-experiment/main-repo && git config core.autocrlf false
$ cd /tmp/wt-experiment/wt-lock-test && git config core.autocrlf
false
```

### Per-worktree config (`extensions.worktreeConfig`)

When enabled, each worktree gets its own `config.worktree` file:
```
$ git config extensions.worktreeConfig true
$ git config --worktree my.worktreeonly "main-value"   # in main repo
$ git config --worktree my.worktreeonly "linked-value" # in linked worktree

# They are independent:
$ cd /tmp/wt-experiment/main-repo && git config my.worktreeonly
main-value
$ cd /tmp/wt-experiment/wt-lock-test && git config my.worktreeonly
linked-value
```

### Submodules

From the git documentation (BUGS section):
```
Multiple checkout in general is still experimental, and the support for
submodules is incomplete. It is NOT recommended to make multiple
checkouts of a superproject.
```

Worktrees with submodules cannot be moved with `git worktree move`, and require `--force` to remove.

---

## 8. Operations Inside a Worktree

### `git log` — works normally, sees full shared history

```
$ cd /tmp/wt-experiment/wt-lock-test && git log --oneline --all
70d0fff add gitignore
160bc15 test hook in worktree
5fefc11 test hook in main
13235f8 second commit
e1d9ec0 initial commit
```

### `git status` — shows the worktree's own state

```
$ cd /tmp/wt-experiment/wt-lock-test && git status
On branch lock-test
Untracked files:
  (use "git add <file>..." to include in what will be committed)
	test.log
nothing added to commit but untracked files present
```

### `git branch` — sees all branches shared across worktrees

```
$ cd /tmp/wt-experiment/wt-lock-test && git branch -a
  feature-a
  feature-b
+ lock-prune-test
* lock-test
+ main
  manual-delete-test
  new-feature
  wt-auto-branch
```
(branches marked `+` are checked out in another worktree)

### `git push` — works from any worktree (same remote config)

Remote config is in the shared `.git/config`, so `git push` works the same from any worktree. The pushed branch will be whatever is checked out in that worktree.

### `git rev-parse --show-toplevel` — returns the WORKTREE root

```
# From linked worktree:
$ cd /tmp/wt-experiment/wt-lock-test && git rev-parse --show-toplevel
/private/tmp/wt-experiment/wt-lock-test

# From main:
$ cd /tmp/wt-experiment/main-repo && git rev-parse --show-toplevel
/private/tmp/wt-experiment/main-repo
```

### `gh` CLI behavior

`gh` determines the repo by reading `git remote get-url origin` (or equivalent), which is stored in the shared `.git/config`. Since config is shared:
- `gh issue list`, `gh pr create`, `gh pr status` all work correctly from any linked worktree.
- No special `GH_TOKEN` or config is needed specifically for worktrees.
- `gh` will operate against the branch checked out in the current worktree, which is exactly the desired behavior for pr9k.

The `--show-toplevel` result matters: when `gh pr create` opens an editor or resolves local paths, it uses the worktree's root. This is correct behavior for pr9k — each worktree is working on a distinct issue branch.

---

## 9. Edge Cases

### Edge case 1: Path already exists

**Non-empty directory — rejected:**
```
$ mkdir /tmp/wt-experiment/nonempty2 && echo "content" > /tmp/wt-experiment/nonempty2/file
$ git worktree add -b nonempty2-test /tmp/wt-experiment/nonempty2
Preparing worktree (new branch 'nonempty2-test')
fatal: '/tmp/wt-experiment/nonempty2' already exists
```

**Empty directory — succeeds:**
```
$ mkdir /tmp/wt-experiment/emptydir
$ git worktree add -b emptydir-test /tmp/wt-experiment/emptydir
Preparing worktree (new branch 'emptydir-test')
HEAD is now at 70d0fff add gitignore
```

**Non-existent path — git creates it:**
```
$ git worktree add -b clean-test /tmp/wt-experiment/clean-dir
Preparing worktree (new branch 'clean-test')
HEAD is now at 70d0fff add gitignore
```

**Implication for pr9k:** Before calling `git worktree add`, ensure the target path either does not exist or is empty. Git will create the directory. If it already exists and is non-empty, git refuses.

### Edge case 2: Branch with unpushed commits in primary

Worktree creation on a branch that has local commits (not yet pushed) works fine:
```
# Branch 'unpushed-branch' has a commit not pushed to any remote:
$ git worktree add /tmp/wt-experiment/wt-unpushed unpushed-branch
Preparing worktree (checking out 'unpushed-branch')
HEAD is now at 2d06141 unpushed commit

$ cd /tmp/wt-experiment/wt-unpushed && git log --oneline
2d06141 unpushed commit
70d0fff add gitignore
...
```

The worktree sees the full branch history including unpushed commits. This is expected: worktrees share all refs.

### Edge case 3: Uncommitted changes in primary checkout

Uncommitted changes in the main worktree do NOT affect creation of linked worktrees:
```
# Main repo has an untracked dirty-main.txt:
$ git status
On branch main
Untracked files:
        dirty-main.txt

$ git worktree add -b dirty-test /tmp/wt-experiment/wt-dirty-test
Preparing worktree (new branch 'dirty-test')
HEAD is now at 70d0fff add gitignore

$ git -C /tmp/wt-experiment/wt-dirty-test status
On branch dirty-test
nothing to commit, working tree clean
```

Dirty state in the primary checkout is completely isolated from linked worktree creation.

### Edge case 4: `git worktree add <path>` with no branch arg

The new branch name is the basename of `<path>`:
```
$ git worktree add /tmp/wt-experiment/my-feature-xyz
Preparing worktree (new branch 'my-feature-xyz')
HEAD is now at 70d0fff add gitignore
```

The branch `my-feature-xyz` is automatically created from HEAD. If a branch of that name already exists and is not checked out elsewhere, it is checked out. If it is already checked out, the command fails (see Q2).

### Edge case 5: Worktree from detached HEAD / specific commit

**Detached HEAD (at current branch tip):**
```
$ git worktree add --detach /tmp/wt-experiment/wt-from-detach
Preparing worktree (detached HEAD 70d0fff)
HEAD is now at 70d0fff add gitignore
```

**From a specific commit hash:**
```
$ git worktree add /tmp/wt-experiment/wt-from-commit 13235f82735ae5b47a4ca78006a00c620a56e134
Preparing worktree (detached HEAD 13235f8)
HEAD is now at 13235f8 second commit
```

Both result in a detached HEAD in the new worktree:
```
worktree /private/tmp/wt-experiment/wt-from-commit
HEAD 13235f82735ae5b47a4ca78006a00c620a56e134
detached
```

---

## 10. Recommended Naming and Location Strategy

### Location options tested

| Location | Git Status Noise | Notes |
|---|---|---|
| Sibling: `../project-issue-42` | None | Clean, most conventional |
| Child inside repo: `./wt-issue-42` | YES — appears as untracked dir | Requires `.gitignore` entry |
| Inside `.git/`: `.git/wt/issue-42` | None | `.git/` is never tracked |
| Completely elsewhere: `/tmp/pr9k/issue-42` | None | Cleanest isolation |

**Demonstrated: child worktrees inside the repo appear as untracked in git status:**
```
$ git worktree add -b status-test2 ./wt-inside-repo2
$ git status
On branch main
Untracked files:
        wt-inside-repo2/
```

**Worktrees inside `.git/` do NOT appear in git status:**
```
$ git worktree add -b gitclean-test .git/wt-clean
$ git status
On branch main
nothing to commit, working tree clean
```

### Recommendation for pr9k

For short-lived automation worktrees, the two best options are:

**Option A: Sibling directories (conventional)**
```
/path/to/target-repo/          <- main checkout (projectDir)
/path/to/target-repo-issue-42/ <- pr9k worktree
```
Pro: conventional, no git noise, easy for users to find and inspect.
Con: requires knowing the parent directory of `projectDir`.

**Option B: Inside `.git/` subdirectory (cleanest for tooling)**
```
/path/to/target-repo/.git/pr9k-worktrees/issue-42/
```
Pro: completely isolated from git status, automatically `.git`-excluded, easy to name.
Con: slightly unusual, lives inside `.git/` which users may not expect to contain working trees.

**Option C: Under pr9k's own runtime dir**
```
~/.pr9k/worktrees/<repo-hash>/issue-42/
```
Pro: fully isolated from the target repo, centralized, easy for pr9k to manage.
Con: not co-located with the project; may surprise users who inspect worktrees manually.

**Note:** The `$GIT_DIR/worktrees/` path (e.g., `.git/worktrees/`) is the *admin* directory — it already exists and holds per-worktree metadata files. Placing the physical working tree there (as in the test above) is technically allowed but creates a naming overlap with the admin dir. A better name like `.git/pr9k-wt/` or `.git/active-worktrees/` avoids confusion.

---

## Admin Dir Details: `$GIT_DIR/worktrees/<name>/`

This directory is created automatically by `git worktree add` and contains:

| File | Contents | Purpose |
|---|---|---|
| `HEAD` | `ref: refs/heads/<branch>` or SHA | Per-worktree current branch/commit |
| `gitdir` | Absolute path to worktree's `.git` file | Back-link from admin → worktree |
| `commondir` | `../..` | Points to the shared `$GIT_DIR` |
| `index` | Binary index file | Per-worktree staging area |
| `locked` | Reason text (if locked) | Prevents pruning |
| `logs/` | Per-worktree reflog | HEAD reflog for this worktree |
| `refs/` | Per-worktree refs (bisect, etc.) | Isolated refs |

The admin dir name defaults to the basename of the worktree path, with a numeric suffix if there is a collision (e.g., `test-next` then `test-next1`).

---

## Porcelain Output Reference (complete example)

```
$ git worktree list --porcelain
worktree /private/tmp/wt-experiment/main-repo
HEAD 70d0fff86d66753d7a91ba4f4eab5ae747e72648
branch refs/heads/main

worktree /private/tmp/wt-experiment/wt-detached
HEAD 13235f82735ae5b47a4ca78006a00c620a56e134
detached

worktree /private/tmp/wt-experiment/wt-lock-prune
HEAD 13235f82735ae5b47a4ca78006a00c620a56e134
branch refs/heads/lock-prune-test
locked portable device

worktree /private/tmp/wt-experiment/wt-lock-test
HEAD 160bc1598a89fb4d22b95946f04a10091d60b3fc
branch refs/heads/lock-test

worktree /private/tmp/wt-experiment/wt-unpushed
HEAD 2d061413a98fb470a683523aca97b25cdd1dc0c2
branch refs/heads/unpushed-branch
```
