package main

import "testing"

// The binary must at minimum report a version, so that the build acceptance
// criteria have something to execute.
func TestVersionDefault(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty")
	}
}
