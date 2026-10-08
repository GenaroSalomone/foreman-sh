# The demo lane

A lane is one product repository with a brainer of its own. This one exists to
prove an install end to end on a repository you do not care about. The toy
repository holds one file, a one-line README, because the example brief asks the
executor to read the README.

```sh
git init ~/code/toy && echo "toy: a throwaway repository for trying foreman-sh." > ~/code/toy/README.md
git -C ~/code/toy add README.md && git -C ~/code/toy commit -m init
./install.sh --brain ~/brain --lane demo --repo ~/code/toy
cp examples/demo/briefs/hello.md ~/brain/demo/briefs/
brain demo                                   # then, from the brainer's pane:
hw demo hello --brief ~/brain/demo/briefs/hello.md --sdd none --no-report --dry-run
hw demo hello --brief ~/brain/demo/briefs/hello.md --sdd none
```

The dry run carries `--no-report` because `hw` exits 1 with `HW_INVOKER_PANE UNRESOLVED` outside a
herdr pane that `brain` opened; the launch line has none. It prints the whole dispatch — worktree, branch, base, placement,
agent — and creates nothing. The second command opens an executor in its own
herdr tab, on its own worktree of `~/code/toy`, which ends by reporting to the
`demo` brainer with `done-invoker`.
