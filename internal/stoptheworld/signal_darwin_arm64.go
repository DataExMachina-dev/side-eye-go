//go:build darwin && arm64

package stoptheworld

// Used in assembly.
//
// These are the flags to set on out sigactiont struct in setHandler.
// See https://github.com/golang/go/blob/45446c86/src/runtime/os_darwin.go
const saFlags = _SA_SIGINFO | _SA_ONSTACK | _SA_RESTART

//go:cgo_import_dynamic libc__NSGetExecutablePath _NSGetExecutablePath "/usr/lib/libSystem.B.dylib"

func nsGetExecutablePath(buf *byte, bufsize *uint32) int32

type sigaction struct {
	prevSigsegv usigactiont
	prevSigbus  usigactiont
}
