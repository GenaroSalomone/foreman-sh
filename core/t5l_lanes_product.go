package main

// The export carries one lane builder, git-worktree.sh. A hook of any other name is bash's to source
// (hw-core hands it back with HW_IMPL=bash), so this answers false.

func (l *lch) laneBuildProduct(base string) bool { return false }
