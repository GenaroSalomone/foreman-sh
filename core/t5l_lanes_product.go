package main

// The export carries one lane builder, git-worktree.sh. A hook of any other name is refused by refuseForeignHook
// (hw has no bash left to source it into), so this answers false.

func (l *lch) laneBuildProduct(base string) bool { return false }
