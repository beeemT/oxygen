package output

import (
	"fmt"
	"os"
	"strings"
)

// PromptPassword reads a password from stdin.
// On Unix it reads from /dev/tty if available to avoid consuming piped stdin.
// Note: terminal echo suppression requires terminal configuration by the caller.
func PromptPassword(message string) (string, error) {
	fmt.Print(message)

	// Try reading from /dev/tty first so piped stdin is not consumed.
	f := os.Stdin
	tty, err := os.Open("/dev/tty")
	if err == nil {
		f = tty
		defer func() { _ = tty.Close() }()
	}

	pw, err := readLine(f)
	if err != nil {
		return "", err
	}
	fmt.Println()

	return strings.TrimRight(pw, "\r"), nil
}

// readLine reads a line from the given file. No terminal control is applied;
// for password prompting the caller should configure the terminal separately.
func readLine(f *os.File) (string, error) {
	var s string
	_, err := fmt.Fscanln(f, &s)

	return s, err
}
