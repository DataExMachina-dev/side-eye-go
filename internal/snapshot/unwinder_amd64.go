//go:build amd64

package snapshot

// adjustCFA adjust the base pointer value to below the return address.
//
// See https://github.com/golang/go/blob/94982a07/src/cmd/compile/abi-internal.md?plain=1#L448-L473
func adjustCFA(
	fps []uintptr,
	// The stack top value is not used on amd64.
	_ uintptr,
) []uintptr {
	for i := range fps {
		fps[i] += 16
	}
	return fps
}
