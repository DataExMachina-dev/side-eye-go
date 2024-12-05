//go:build !(darwin && arm64) && !(linux && amd64)

package stoptheworld

type sigaction struct{}

// OsArchSupported returns whether the combination of OS and architecture are
// supported.
func OsArchSupported() bool {
	return false
}
