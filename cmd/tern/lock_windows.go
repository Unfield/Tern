//go:build windows

package main

import "os"

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid) // fails on Windows if the process does not exist
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
