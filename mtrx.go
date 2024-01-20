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

type Metrics struct {
	time struct {
		user  float64
		sys   float64
		clock float64
	}

	sys struct {
		maxRss     int64
		pageFaults struct {
			minor int64
			major int64
		}
	}

	fs struct {
		input  int64
		output int64
	}

	contextSwitch struct {
		involuntarily int64
		voluntarily   int64
	}

	exit struct {
		status     int
		signal     int
		stopSignal int
		trapCause  int

		exited    bool
		signaled  bool
		stopped   bool
		continued bool
		coreDump  bool
	}
}

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
	metrics := newMetrics(&waitStatus, &rusage, elapsed)
	summary(metrics)

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
func summary(metrics Metrics) {
	summaryField("time.user", "%0.3fs", metrics.time.user)
	summaryField("time.sys", "%0.3fs", metrics.time.sys)
	summaryField("time.clock", "%0.3fs", metrics.time.clock)
	fmt.Println()

	summaryField("sys.max_rss", "%v KiB", metrics.sys.maxRss)
	summaryField("sys.page_faults.minor", "%v", metrics.sys.pageFaults.minor)
	summaryField("sys.page_faults.major", "%v", metrics.sys.pageFaults.major)
	fmt.Println()

	summaryField("sys.fs.input", "%v", metrics.fs.input)
	summaryField("sys.fs.output", "%v", metrics.fs.output)
	fmt.Println()

	summaryField("sys.context_switch.involuntarily", "%v", metrics.contextSwitch.involuntarily)
	summaryField("sys.context_switch.voluntarily", "%v", metrics.contextSwitch.voluntarily)
	fmt.Println()

	summaryField("exit.status", "%v", metrics.exit.status)
	if metrics.exit.signaled {
		summaryField("exit.signal", "%v", metrics.exit.signal)
	}

	if metrics.exit.stopped {
		summaryField("exit.stop_signal", "%v", metrics.exit.stopSignal)
	}

	// I'm not sure that continued and trapCause are related
	if metrics.exit.continued {
		summaryField("exit.trap_cause", "%v", metrics.exit.trapCause)
	}

	summaryField("exit.exited", "%v", metrics.exit.exited)
	summaryField("exit.signaled", "%v", metrics.exit.signaled)
	summaryField("exit.stopped", "%v", metrics.exit.stopped)
	summaryField("exit.continued", "%v", metrics.exit.continued)
	summaryField("exit.core_dump", "%v", metrics.exit.coreDump)
}

func summaryField(field string, format string, value any) {
	valueFormatted := fmt.Sprintf(format, value)
	fmt.Printf("%-35s %10s\n", field+":", valueFormatted)
}

func newMetrics(waitStatus *syscall.WaitStatus, rusage *syscall.Rusage, elapsed time.Duration) Metrics {
	metrics := Metrics{}
	metrics.time.user = microToMillis(rusage.Utime)
	metrics.time.sys = microToMillis(rusage.Stime)
	metrics.time.clock = elapsed.Seconds()

	metrics.sys.maxRss = rusage.Maxrss
	metrics.sys.pageFaults.minor = rusage.Minflt
	metrics.sys.pageFaults.major = rusage.Majflt

	metrics.fs.input = rusage.Inblock
	metrics.fs.output = rusage.Oublock

	metrics.contextSwitch.involuntarily = rusage.Nivcsw
	metrics.contextSwitch.voluntarily = rusage.Nvcsw

	metrics.exit.status = waitStatus.ExitStatus()
	metrics.exit.signal = int(waitStatus.Signal())
	metrics.exit.stopSignal = int(waitStatus.StopSignal())
	metrics.exit.trapCause = waitStatus.TrapCause()

	metrics.exit.exited = waitStatus.Exited()
	metrics.exit.signaled = waitStatus.Signaled()
	metrics.exit.stopped = waitStatus.Stopped()
	metrics.exit.continued = waitStatus.Continued()
	metrics.exit.coreDump = waitStatus.CoreDump()

	return metrics
}
