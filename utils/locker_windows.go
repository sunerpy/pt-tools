//go:build windows
// +build windows

//
package utils

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type fileLocker struct {
	file *os.File
}

func NewLocker(lockPath string) (Locker, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &fileLocker{file: f}, nil
}

func (l *fileLocker) Lock() error {
	var ol windows.Overlapped
	handle := windows.Handle(l.file.Fd())
	err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if err != nil {
		return fmt.Errorf("lock failed: %w", err)
	}
	return nil
}

func (l *fileLocker) Unlock() error {
	var ol windows.Overlapped
	handle := windows.Handle(l.file.Fd())
	err := windows.UnlockFileEx(handle, 0, 1, 0, &ol)
	if err != nil {
		return fmt.Errorf("unlock failed: %w", err)
	}
	return nil
}

func (l *fileLocker) File() *os.File {
	return l.file
}
