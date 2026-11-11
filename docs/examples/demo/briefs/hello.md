---
kind: build
requires: repo
---

# hello

## Objective

Add a file `HELLO.md` at the root of the repository with one line: what this
repository is, in your own words, after reading its README.

## What is not done

- Nothing else in the repository changes.

**Framework:** `--sdd none`.

## Verification

```
test -s HELLO.md
```

Commit it on your branch, then report with `done-invoker`, naming the commit.
