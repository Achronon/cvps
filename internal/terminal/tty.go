package terminal

import (
	"os"

	"golang.org/x/term"
)

// GetSize returns the current terminal size
func GetSize() (cols, rows int, err error) {
	cols, rows, err = term.GetSize(int(os.Stdout.Fd()))
	return
}

// SetRaw puts the terminal in raw mode and returns a restore function
func SetRaw() (restore func(), err error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}

	return func() {
		term.Restore(fd, oldState)
	}, nil
}
