package native

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// outputDirectory keeps the acquired directory and any platform creation guard alive.
type outputDirectory struct {
	*os.Root
	guard *os.File
}

// OutputTarget binds a caller-checked parent and single basename. The caller
// retains ownership of Parent; native output never reopens its pathname.
type OutputTarget struct {
	Parent *os.Root
	Name   string
}

func (d *outputDirectory) Close() error {
	err := d.Root.Close()
	if d.guard != nil {
		err = errors.Join(err, d.guard.Close())
	}
	return err
}

func recordCleanupFailure(report *kit.Report, resultErr *error, code string, cause error) {
	if report.ExecutionStatus != kit.StatusCancelled && report.ExecutionStatus != kit.StatusResourceLimit {
		report.ExecutionStatus = kit.StatusFailed
	}
	if report.Assessment == kit.AssessPass {
		report.Assessment = kit.AssessNotAssessed
	}
	report.Coverage.Observed = []string{}
	report.Findings = append(report.Findings, finding(code, "error", "", "build 또는 출력 경로의 자원을 해제하지 못했다"))
	*resultErr = errors.Join(*resultErr, refuse(kit.KindIO, code, cause))
}

func closeOutput(d *outputDirectory, report *kit.Report, resultErr *error) {
	if err := d.Close(); err != nil {
		recordCleanupFailure(report, resultErr, "OUTPUT_CLOSE_FAILED", err)
	}
}

func newOutputDirectory(path string, target *OutputTarget) (directory *outputDirectory, resultErr error) {
	if target != nil {
		if target.Parent == nil || !filepath.IsLocal(target.Name) || filepath.Base(target.Name) != target.Name || target.Name == "." {
			return nil, os.ErrInvalid
		}
		return createOutputDirectory(target.Parent, target.Name)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(abs); err == nil {
		return nil, os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	parent, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := parent.Close(); err != nil {
			if directory != nil {
				err = errors.Join(err, directory.Close())
				directory = nil
			}
			resultErr = errors.Join(resultErr, err)
		}
	}()
	return createOutputDirectory(parent, filepath.Base(abs))
}

// createExclusive is shared by incremental and oracle output; failed acquisition
// never grants permission to write, truncate or clean up another owner's entry.
var createExclusive = func(root *os.Root, name string, data []byte) error {
	if root == nil {
		return os.ErrInvalid
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}
