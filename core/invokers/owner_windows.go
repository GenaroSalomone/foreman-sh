package invokers

import "os"

// ownedByOther: Windows has no uid to compare, so no file is reported as another user's.
func ownedByOther(os.FileInfo) bool { return false }
