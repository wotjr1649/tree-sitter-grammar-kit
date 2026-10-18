//go:build !windows

package native

import "os"

func createOutputDirectory(parent *os.Root, name string) (*outputDirectory, error) {
	if err := parent.Mkdir(name, 0o755); err != nil {
		return nil, err
	}
	// shortcut: mkdir-to-open ownership is unproven under concurrent replacement;
	// retain #215 until an atomic identity acquisition mechanism is available.
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &outputDirectory{Root: root}, nil
}
