//go:build !windows && !linux

package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
)

func publishOutput(destination *os.Root, final string, data []byte) (code string, resultErr error) {
	name := ".tsgk-out-" + rand.Text() + ".tmp"
	tmp, err := destination.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	linked := false
	defer func() {
		if err := destination.Remove(name); err != nil {
			if linked {
				err = fmt.Errorf("output was published; temporary cleanup failed: %w", err)
			}
			resultErr = errors.Join(resultErr, err)
			code = "OUTPUT_CLEANUP_FAILED"
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return "OUTPUT_PUBLISH_FAILED", errors.Join(err, tmp.Close())
	}
	if err := tmp.Sync(); err != nil {
		return "OUTPUT_PUBLISH_FAILED", errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	if testHookBeforeLink != nil {
		testHookBeforeLink()
	}
	if err := destination.Link(name, final); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "OUTPUT_EXISTS", err
		}
		return "OUTPUT_PUBLISH_FAILED", err
	}
	linked = true
	if testHookAfterLink != nil {
		testHookAfterLink(nil)
	}
	return "", nil
}
