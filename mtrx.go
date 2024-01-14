package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// This is what `time` and `sh` return for processes that exit with a signal.
// Their return value is: SIGNAL + SIGNALLED_OFFSET
const SIGNALLED_OFFSET = 128

func main() {
	if len(os.Args) == 1 {
		fmt.Printf("Usage: %v CMD [ARG]...\n", filepath.Base(os.Args[0]))
		return
	}
	var err error

	// Command to start
	cmd := os.Args[1:]

	// Find the executable's full path
	path, err := exec.LookPath(cmd[0])
	if err != nil {
		log.Fatal(err)
	}

	// Get the current folder
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Build the process attribute
	procAttr := syscall.ProcAttr{
		Dir:   pwd,
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		Sys:   nil,
	}

	// Start the child process
	start := time.Now()
	pid, err := syscall.ForkExec(path, cmd, &procAttr)

	// Block all signals for the parent process, but let the child handle them
	signal.Ignore(syscall.SIGINT, syscall.SIGQUIT)

	// Wait for the child process
	var waitStatus syscall.WaitStatus
	var rusage syscall.Rusage
	if child_pid, err := waitPid(pid, &waitStatus, &rusage); err != nil {
		log.Fatal(err)
	} else if child_pid != pid {
		log.Fatalf("Waited for PID %v but got PID %v\n", pid, child_pid)
	}
	elapsed := time.Now().Sub(start)

	// Display a summary
	summary(&waitStatus, &rusage, elapsed)

	// Propagate the child's exit status
	var status = 0
	switch {
	case waitStatus.Stopped():
		status = int(waitStatus.StopSignal()) + SIGNALLED_OFFSET
	case waitStatus.Signaled():
		status = int(waitStatus.Signal()) + SIGNALLED_OFFSET
	case waitStatus.Exited():
		status = waitStatus.ExitStatus()
	}

	// Exit with the same exist status as the child process
	os.Exit(status)
}

/**
 * Waits for the given process and returns the status and rusage.
 */
func waitPid(pid int, waitStatus *syscall.WaitStatus, rusage *syscall.Rusage) (childPid int, err error) {
	childPid, err = syscall.Wait4(pid, waitStatus, 0, rusage)
	for err == syscall.EINTR {
		childPid, err = syscall.Wait4(pid, waitStatus, 0, rusage)
	}

	return childPid, err
}

/**
 * Display a summary of the process' execution.
 */
func summary(waitStatus *syscall.WaitStatus, rusage *syscall.Rusage, elapsed time.Duration) {
	fmt.Printf("User CPU time: %v; %v\n", rusage.Utime.Sec, rusage.Utime.Usec)
	fmt.Printf("Sys  CPU time: %v; %v\n", rusage.Stime.Sec, rusage.Stime.Usec)
	fmt.Printf("elapsed %v\n", elapsed)
	fmt.Printf("Exit status = %v\n", waitStatus.ExitStatus())
}
