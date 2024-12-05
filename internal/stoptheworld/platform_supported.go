package stoptheworld

import (
	"errors"
	"fmt"
	"runtime"
)

func PlatformSupported() error {
	if !GoVersionSupported() {
		return fmt.Errorf("go version %s not supported", runtime.Version())
	}
	if !OsArchSupported() {
		return errors.New("OS/architecture combination not supported")
	}
	return nil
}
