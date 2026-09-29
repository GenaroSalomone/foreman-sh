# The demo lane

A lane is one product repository with a brainer of its own. This one exists to
prove an install end to end on a repository you do not care about.

```sh
git init ~/code/toy && git -C ~/code/toy commit --allow-empty -m init
./install.sh --brain ~/brain --lane demo --repo ~/code/toy
cp examples/demo/briefs/hello.md ~/brain/demo/briefs/
brain demo                                   # then, from the brainer's pane:
hw demo hello --brief ~/brain/demo/briefs/hello.md --sdd none --dry-run
hw demo hello --brief ~/brain/demo/briefs/hello.md --sdd none
```

The dry run prints the whole dispatch — worktree, branch, base, placement,
agent — and creates nothing. The second command opens an executor in its own
herdr tab, on its own worktree of `~/code/toy`, which ends by reporting to the
`demo` brainer with `done-invoker`.
