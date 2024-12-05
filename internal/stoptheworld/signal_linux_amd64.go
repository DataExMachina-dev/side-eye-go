//go:build linux && amd64

package stoptheworld

// Used in assembly.
//
// These are the flags to set on out sigactiont struct in setHandler.
const saFlags = _SA_ONSTACK | _SA_SIGINFO | _SA_NODEFER | _SA_RESTORER

type sigaction = sigactiont

// OsArchSupported returns whether the combination of OS and architecture are
// supported.
func OsArchSupported() bool {
	return true
}
