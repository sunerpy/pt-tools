//go:build linux || darwin
// +build linux darwin

//
package utils

import (
	"os"

	"golang.org/x/sys/unix"
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
	return unix.Flock(int(l.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func (l *fileLocker) Unlock() error {
	return unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
}

func (l *fileLocker) File() *os.File {
	return l.file
}
