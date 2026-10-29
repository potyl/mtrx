package mtrxtypes

type Metrics struct {
	Time struct {
		User  float64
		Sys   float64
		Clock float64
	}

	Sys struct {
		// Maximum resident set size used (in kilobytes)
		MaxRss     int64
		PageFaults struct {
			Minor int64
			Major int64
		}
	}

	// Bytes that the file system read from and wrote to the storage (excludes the page cache).
	FsIO struct {
		Read  int64
		Write int64
	}

	ContextSwitch struct {
		Involuntarily int64
		Voluntarily   int64
	}

	WaitStatus struct {
		Status     int
		Signal     int
		StopSignal int
		TrapCause  int

		Exited    bool
		Signaled  bool
		Stopped   bool
		Continued bool
		CoreDump  bool
	}

	ExitCode int
}

type Process struct {
	*Metrics
	PID         int
	ParentPID   int
	Command     string
	CommandArgs []string
	Owner       string
	Executable
}

type Executable struct {
	Name string
	Path string
}
