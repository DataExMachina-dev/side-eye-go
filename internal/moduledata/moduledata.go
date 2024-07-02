// Package moduledata contains logic to locate the firstmoduledata variable.
package moduledata

import (
	"runtime"
	"unsafe"
)

// runtime.findfunc returns a funcInfo which has a pointer to the relevant
// runtime.moduledata.
//
// See https://github.com/golang/go/blob/cdbf5f2f/src/runtime/symtab.go#L857-L873
//
//go:linkname runtimeFindfunc runtime.findfunc
func runtimeFindfunc(pc uintptr) funcInfo

// This corresponds to the runtime.funcInfo structure.
//
// See https://github.com/golang/go/blob/cdbf5f2f/src/runtime/symtab.go#L823-L826
type funcInfo struct {
	_     uintptr
	datap unsafe.Pointer
}

// GetFirstmoduledata returns the address of the firstmoduledata variable.
func GetFirstmoduledata() unsafe.Pointer {
	// The approach here is somewhat hacky, but it works and should remain
	// stable because the function has been marked as a stable runtime API.
	//
	// The observation is that the runtime.findfunc function returns a structure
	// with a pointer to the runtime.moduledata for the function being looked
	// up. Given our library is linked into the binary, we can use that to find
	// runtime.firstmoduledata.
	pc, _, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	fi := runtimeFindfunc(pc)
	return fi.datap
}
