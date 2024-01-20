package mtrxproc

import (
	"mtrx/internal/mtrxexec"
	"mtrx/internal/mtrxtypes"

	"os/exec"
	"os/user"
	"syscall"
	"time"
)

// Executes the given command, waits for the process and collects instrumentation data about the child process.
func InstrumentProcess(cmd []string) (*mtrxtypes.Process, error) {
	// Find the executable's full path
	path, err := exec.LookPath(cmd[0])
	if err != nil {
		return nil, err
	}

	// Start the child process
	process, err := mtrxexec.TimeForkExec(path, cmd)
	if err != nil {
		return nil, err
	}

	// Generate the metrics
	metrics := newMetrics(process.WaitStatus, process.Rusage, process.Elapsed)

	user, err := user.Current()
	if err != nil {
		return nil, err
	}

	mProcess := mtrxtypes.Process{
		Metrics:     metrics,
		PID:         process.Pid,
		ParentPID:   syscall.Getppid(), // Let's pretend that our parent is the process' parent
		Command:     cmd[0],
		CommandArgs: cmd[1:],
		Owner:       user.Username,

		Executable: mtrxtypes.Executable{
			Name: cmd[0],
			Path: path,
		},
	}

	return &mProcess, nil
}

// Creates a Metrics instance based on the given input.
func newMetrics(waitStatus *syscall.WaitStatus, rusage *syscall.Rusage, elapsed time.Duration) *mtrxtypes.Metrics {
	metrics := mtrxtypes.Metrics{}
	metrics.Time.User = microToMillis(rusage.Utime)
	metrics.Time.Sys = microToMillis(rusage.Stime)
	metrics.Time.Clock = elapsed.Seconds()

	// `rusage` has the data in kilobytes
	metrics.Sys.MaxRss = rusage.Maxrss

	metrics.Sys.PageFaults.Minor = rusage.Minflt
	metrics.Sys.PageFaults.Major = rusage.Majflt

	// `rusage` has the data in blocks, a block is usually 512 bytes, but it can change per device.
	// The problem is that we don't know from which device the process made the I/O.
	metrics.FsBlocks.Input = rusage.Inblock
	metrics.FsBlocks.Output = rusage.Oublock

	metrics.ContextSwitch.Involuntarily = rusage.Nivcsw
	metrics.ContextSwitch.Voluntarily = rusage.Nvcsw

	metrics.WaitStatus.Status = waitStatus.ExitStatus()
	metrics.WaitStatus.Signal = int(waitStatus.Signal())
	metrics.WaitStatus.StopSignal = int(waitStatus.StopSignal())
	metrics.WaitStatus.TrapCause = waitStatus.TrapCause()

	metrics.WaitStatus.Exited = waitStatus.Exited()
	metrics.WaitStatus.Signaled = waitStatus.Signaled()
	metrics.WaitStatus.Stopped = waitStatus.Stopped()
	metrics.WaitStatus.Continued = waitStatus.Continued()
	metrics.WaitStatus.CoreDump = waitStatus.CoreDump()

	metrics.ExitCode = exitCode((waitStatus))

	return &metrics
}

// Converts a micro seconds value provided by the given syscall.TimeVal
// into an float value that's in milliseconds.
func microToMillis(time syscall.Timeval) float64 {
	return float64(time.Sec) + (float64(time.Usec) / float64(1_000_000))
}

// Returns the exit code that a process should use in order to mimic the exit code corresponding to the given WaitStatus.
func exitCode(waitStatus *syscall.WaitStatus) int {
	// This is what `time` and `sh` return for processes that exit with a signal.
	// Their return value is: SIGNAL + SIGNALLED_OFFSET
	const SIGNALLED_OFFSET = 128

	switch {
	case waitStatus.Stopped():
		return int(waitStatus.StopSignal()) + SIGNALLED_OFFSET
	case waitStatus.Signaled():
		return int(waitStatus.Signal()) + SIGNALLED_OFFSET
	case waitStatus.Exited():
		return waitStatus.ExitStatus()
	}

	// This shouldn't happen
	return -1
}
