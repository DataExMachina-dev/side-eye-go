//go:build amd64

package snapshot

func adjustCFA(fps []uintptr, stackTopSp uintptr) []uintptr {
	for i := range fps {
		fps[i] += 16
	}
	return fps
}
