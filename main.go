package main

import "mtrx/cmd"

// The release, set at build time with -ldflags "-X main.version=..."
var version string

func main() {
	cmd.Execute(version)
}
