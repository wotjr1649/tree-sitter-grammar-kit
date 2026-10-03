package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Job Object message identifiers and structures that x/sys does not define
// (winnt.h JOB_OBJECT_MSG_*, JOBOBJECT_ASSOCIATE_COMPLETION_PORT,
// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION).
const (
	jobMsgJobMemoryLimit     = 10
	jobMsgProcessMemoryLimit = 9
)

type jobCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// jobTree places the process in a new Job Object before its first instruction: the
// process starts suspended, is assigned, then resumed. Every descendant inherits the job
// (breakaway is not allowed), so TerminateJobObject reaches escaped descendants as well.
// JobMemoryLimit is a kernel-enforced cap on committed memory of the whole job.
type jobTree struct {
	job, port windows.Handle
	events    chan struct{}
	hit       bool
	mu        sync.Mutex
	wg        sync.WaitGroup
}

func newTree(s *Spec, probe bool) (tree, Capabilities, error) {
	caps := baseCaps()
	caps.Backend, caps.TreeCleanup, caps.EscapedDescendants = "windows-job-object", "JOB_OBJECT", Contained
	caps.Memory, caps.MemoryMetric = MemoryHard, "job committed memory (JobMemoryLimit/PeakJobMemoryUsed, bytes)"
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		caps.Memory = MemoryUnsupported
		return nil, caps, &Error{Code: "BACKEND_UNAVAILABLE", Blocked: true, Cause: err}
	}
	t := &jobTree{job: job, events: make(chan struct{}, 1)}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION | windows.JOB_OBJECT_LIMIT_JOB_MEMORY
	info.JobMemoryLimit = uintptr(s.Memory.Bytes)
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		caps.Memory = MemoryUnsupported
		return nil, caps, &Error{Code: "BACKEND_UNAVAILABLE", Blocked: true, Cause: err}
	}
	if probe {
		return t, caps, nil
	}
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		windows.CloseHandle(job)
		return nil, caps, &Error{Code: "BACKEND_UNAVAILABLE", Blocked: true, Cause: err}
	}
	assoc := jobCompletionPort{CompletionKey: 1, CompletionPort: port}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation, uintptr(unsafe.Pointer(&assoc)), uint32(unsafe.Sizeof(assoc))); err != nil {
		windows.CloseHandle(port)
		windows.CloseHandle(job)
		return nil, caps, &Error{Code: "BACKEND_UNAVAILABLE", Blocked: true, Cause: err}
	}
	t.port = port
	t.wg.Add(1)
	go t.listen()
	return t, caps, nil
}

// listen receives job notifications until the port is closed.
func (t *jobTree) listen() {
	defer t.wg.Done()
	for {
		var msg uint32
		var key uintptr
		var ov *windows.Overlapped
		if err := windows.GetQueuedCompletionStatus(t.port, &msg, &key, &ov, windows.INFINITE); err != nil {
			return
		}
		if msg == jobMsgJobMemoryLimit || msg == jobMsgProcessMemoryLimit {
			t.mu.Lock()
			t.hit = true
			t.mu.Unlock()
			select {
			case t.events <- struct{}{}:
			default:
			}
		}
	}
}

func (t *jobTree) prepare(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	return nil
}

func (t *jobTree) attach(p *os.Process) error {
	pid := uint32(p.Pid)
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(t.job, h); err != nil {
		return fmt.Errorf("assign job: %w", err)
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	resumed := 0
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != pid {
			continue
		}
		th, oerr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
		if oerr != nil {
			return oerr
		}
		_, rerr := windows.ResumeThread(th)
		windows.CloseHandle(th)
		if rerr != nil {
			return rerr
		}
		resumed++
	}
	if resumed == 0 {
		return errors.New("no thread of the suspended process to resume")
	}
	return nil
}

func (t *jobTree) stop(bool) { windows.TerminateJobObject(t.job, 1) }

func (t *jobTree) live() (int, error) {
	var acc jobAccounting
	if err := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&acc)), uint32(unsafe.Sizeof(acc)), nil); err != nil {
		return 0, err
	}
	return int(acc.ActiveProcesses), nil
}

func (t *jobTree) limitHit() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.hit
}

func (t *jobTree) peak() (uint64, bool) {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(t.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return 0, false
	}
	return uint64(info.PeakJobMemoryUsed), true
}

func (t *jobTree) sample() (uint64, error) { return 0, errors.New("not sampled") }

func (t *jobTree) notify() <-chan struct{} { return t.events }

func (t *jobTree) close() error {
	var err error
	if t.port != 0 {
		err = windows.CloseHandle(t.port)
		t.wg.Wait()
	}
	// KILL_ON_JOB_CLOSE ends anything still inside when the last handle closes.
	if cerr := windows.CloseHandle(t.job); err == nil {
		err = cerr
	}
	return err
}
