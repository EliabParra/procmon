package main

import "testing"

func TestControllerForwardsToTheControlPackage(t *testing.T) {
	// Read-only on purpose: a pid that cannot exist must be rejected by package control.
	if _, err := (controller{}).Priority(-1); err == nil {
		t.Error("Priority(-1) must be rejected")
	}
	if _, err := (controller{}).Priority(1); err != nil {
		t.Errorf("Priority(1) = %v, want the nice value of init", err)
	}
}
