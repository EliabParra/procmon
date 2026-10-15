package main

import (
	"io"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    config
		wantErr bool
	}{
		{"defaults", nil, config{interval: time.Second}, false},
		{"interval", []string{"-interval", "2s"}, config{interval: 2 * time.Second}, false},
		{"fractional interval", []string{"-interval=500ms"}, config{interval: 500 * time.Millisecond}, false},
		{"plain", []string{"-plain"}, config{interval: time.Second, plain: true}, false},
		{"too fast", []string{"-interval", "10ms"}, config{}, true},
		{"too slow", []string{"-interval", "5m"}, config{}, true},
		{"not a duration", []string{"-interval", "fast"}, config{}, true},
		{"unknown flag", []string{"-nope"}, config{}, true},
		{"stray argument", []string{"extra"}, config{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args, io.Discard)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("config = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestControllerForwardsToTheControlPackage(t *testing.T) {
	// Read-only on purpose: a pid that cannot exist must be rejected by package control.
	if _, err := (controller{}).Priority(-1); err == nil {
		t.Error("Priority(-1) must be rejected")
	}
	if _, err := (controller{}).Priority(1); err != nil {
		t.Errorf("Priority(1) = %v, want the nice value of init", err)
	}
}
