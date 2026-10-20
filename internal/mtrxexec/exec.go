package mtrxexec

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ProcessResult struct {
	*syscall.WaitStatus
	*syscall.Rusage
	Elapsed time.Duration
	Pid     int
}

// Executes the given command, waits for the process and returns some metrics about the child process.
func TimeForkExec(path string, cmd []string) (*ProcessResult, error) {
	// Build the process attribute
	procAttr := syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
	}

	// Start the child process
	start := time.Now()
	pid, err := syscall.ForkExec(path, cmd, &procAttr)
	if err != nil {
		return nil, err
	}

	// Block all signals for the parent process, but let the child handle them
	signal.Ignore(syscall.SIGINT, syscall.SIGQUIT)

	// Wait for the child process
	var waitStatus syscall.WaitStatus
	var rusage syscall.Rusage
	if _, err := waitPid(pid, &waitStatus, &rusage); err != nil {
		return nil, err
	}
	elapsed := time.Since(start)

	process := ProcessResult{
		WaitStatus: &waitStatus,
		Rusage:     &rusage,
		Elapsed:    elapsed,
		Pid:        pid,
	}

	return &process, nil
}

// Waits for the given process and returns the status and rusage.
func waitPid(pid int, waitStatus *syscall.WaitStatus, rusage *syscall.Rusage) (childPid int, err error) {
	for {
		childPid, err = syscall.Wait4(pid, waitStatus, 0, rusage)
		if err != syscall.EINTR {
			if childPid != pid {
				// This should never happen as we started a single process
				// and thus we can  only wait for the single child.
				message := fmt.Sprintf("Waited for PID %v but got PID %v\n", pid, childPid)
				err = errors.New(message)
			}
			return childPid, err
		}
	}
}
