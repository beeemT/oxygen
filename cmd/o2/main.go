// o2 is the OpenObserve CLI entry point.
package main

import (
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	// Cobra commands that return errors should set the exit code.
	// Default to 1 for unknown errors.
	return 1
}
