//go:build arm64

package snapshot

func adjustCFA(fps []uintptr, stackTopSp uintptr) []uintptr {
	for i := 0; i < len(fps)-1; i++ {
		fps[i] = fps[i+1] + 8
	}
	fps[len(fps)-1] = stackTopSp
	return fps
}
