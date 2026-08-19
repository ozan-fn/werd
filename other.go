//go:build !windows

package main

import "os/exec"

func noWindow(cmd *exec.Cmd) *exec.Cmd { return cmd }

func hideFromTaskbar()                      {}
func phpInUserPath(root string) bool        { return false }
func setPhpInUserPath(root string, on bool) {}
