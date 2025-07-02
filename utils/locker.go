package utils

import "os"

// 定义统一接口
type Locker interface {
	Lock() error
	Unlock() error
	File() *os.File
}
