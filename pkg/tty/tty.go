package tty

import (
	"os"

	"golang.org/x/term"
)

// IsTTY checks if the given file descriptor refers to a terminal.
// If the provided file descriptor is 0, it defaults to checking os.Stdout.
// It returns true if the file descriptor is a terminal, otherwise false.
//
// Parameters:
//
//	fd - The file descriptor to check.
//
// Returns:
//
//	  bool - true if the file descriptor is a terminal, false otherwise.
//		int - The height of the terminal.
//		error - Any error encountered while retrieving the terminal size.
func IsTTY(fd uintptr) bool {
	if fd == 0 {
		fd = os.Stdout.Fd()
	}

	if term.IsTerminal(int(fd)) {
		return true
	}
	return false
}

// Size returns the width and height of the terminal connected to the given file descriptor.
// If the provided file descriptor is 0, it defaults to using the file descriptor of os.Stdout.
// It returns the width, height, and any error encountered while retrieving the terminal size.
//
// Parameters:
//
//	fd - The file descriptor of the terminal.
//
// Returns:
//
//	int - The width of the terminal.
//	int - The height of the terminal.
//	error - Any error encountered while retrieving the terminal size.
func Size(fd uintptr) (int, int, error) {
	if fd == 0 {
		fd = os.Stdout.Fd()
	}

	return term.GetSize(int(fd))
}
