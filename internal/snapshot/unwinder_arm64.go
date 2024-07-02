//go:build arm64

package snapshot

// adjustCFA adjust the base pointer value to point to the frame base for the current
// frame. On arm64, this is actually the next frame's frame pointer. The root frame's
// CFA is the stack top.
//
// See https://github.com/golang/go/blob/94982a07/src/cmd/compile/abi-internal.md?plain=1#L568-L601
func adjustCFA(fps []uintptr, stackTopSp uintptr) []uintptr {
	for i := 0; i < len(fps)-1; i++ {
		fps[i] = fps[i+1] + 8
	}
	fps[len(fps)-1] = stackTopSp
	return fps
}
