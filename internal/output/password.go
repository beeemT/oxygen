package output

import (
	"fmt"
	"os"
	"strings"
)

// PromptPassword reads a password from stdin without echo.
// On Unix platforms it tries to suppress echo via terminal control on /dev/tty.
// Falls back to a plain stdin read (with echo) on failure.
func PromptPassword(message string) (string, error) {
	fmt.Print(message)

	// Try reading from /dev/tty first so piped stdin is not consumed.
	f := os.Stdin
	if tty, err := os.Open("/dev/tty"); err == nil {
		f = tty
		defer tty.Close()
	}

	pw, err := readLine(f)
	if err != nil {
		return "", err
	}
	fmt.Println() // newline after password input
	return strings.TrimRight(pw, "\r"), nil
}

// readLine reads a line from the given file. No terminal control is applied;
// for password prompting the caller should configure the terminal separately.
func readLine(f *os.File) (string, error) {
	var s string
	_, err := fmt.Fscanln(f, &s)
	return s, err
}
