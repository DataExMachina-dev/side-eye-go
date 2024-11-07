//go:build go1.24

package stoptheworld

func GoVersionSupported() bool {
	return false
}

type stwReason int8
type worldStop struct{}

func stopTheWorld(reason stwReason) worldStop {
	return worldStop{}
}

func startTheWorld(ws worldStop) {
}

const (
	stwStartTrace stwReason = iota
)
