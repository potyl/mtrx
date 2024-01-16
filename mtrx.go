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
	if err != nil {
		log.Fatal(err)
	}

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
	elapsed := time.Since(start)

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
	for {
		childPid, err = syscall.Wait4(pid, waitStatus, 0, rusage)
		if err != syscall.EINTR {
			return childPid, err
		}
	}
}

func microToMillis(time syscall.Timeval) float64 {
	t := float64(time.Sec) + (float64(time.Usec) / float64(1_000_000))
	return t
}

/**
 * Display a summary of the process' execution.
 */
func summary(waitStatus *syscall.WaitStatus, rusage *syscall.Rusage, elapsed time.Duration) {
	summaryField("time.user", "%0.3fs", microToMillis(rusage.Utime))
	summaryField("time.sys", "%0.3fs", microToMillis(rusage.Stime))
	summaryField("time.clock", "%0.3fs", elapsed.Seconds())
	fmt.Println()

	summaryField("sys.max_rss", "%v KiB", rusage.Maxrss)
	summaryField("sys.minor_page_faults", "%v", rusage.Minflt)
	summaryField("sys.major_page_faults", "%v", rusage.Majflt)
	fmt.Println()

	summaryField("sys.fs_input", "%v", rusage.Inblock)
	summaryField("sys.fs_output", "%v", rusage.Oublock)
	fmt.Println()

	summaryField("sys.context_switch_involuntarily", "%v", rusage.Nivcsw)
	summaryField("sys.context_switch_voluntarily", "%v", rusage.Nvcsw)
	fmt.Println()

	summaryField("exit.status", "%v", waitStatus.ExitStatus())
}

func summaryField(field string, format string, value any) {
	valueFormatted := fmt.Sprintf(format, value)
	fmt.Printf("%-35s %10s\n", field+":", valueFormatted)
}
