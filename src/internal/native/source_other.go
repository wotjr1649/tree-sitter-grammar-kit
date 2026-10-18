//go:build !windows && !linux && !darwin

package native

import (
	"errors"
	"os"
)

func openSourceLeaf(parent *os.File, name string) (*os.File, error) {
	return nil, errors.ErrUnsupported
}
