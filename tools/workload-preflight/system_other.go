//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package main

import "fmt"

func FreeBytes(path string) (uint64, error) {
	return 0, fmt.Errorf("disk checks are unsupported on this operating system")
}
