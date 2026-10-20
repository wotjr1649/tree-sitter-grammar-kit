//go:build !windows && !linux && !darwin

package native

import (
	"errors"
	"os"
)

func sourceLinks(*os.File) (uint64, error) { return 0, errors.ErrUnsupported }

func openSourceLeaf(parent *os.File, name string) (*os.File, error) {
	return nil, errors.ErrUnsupported
}
