package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

const passwdFixture = `# comment line
root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin

eliab:x:1000:1000:Eliab,,,:/home/eliab:/bin/fish
broken line without colons
short:x:notanumber:5:x:/h:/s
alias:x:1000:1000:second entry for the same uid:/home/alias:/bin/sh
`

func writePasswd(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "passwd")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUsersName(t *testing.T) {
	users := NewUsers(writePasswd(t, passwdFixture))
	tests := []struct {
		name string
		uid  int
		want string
	}{
		{"root", 0, "root"},
		{"regular user", 1000, "eliab"},
		{"first entry wins for a repeated uid", 1000, "eliab"},
		{"unknown uid is printed as a number", 4242, "4242"},
		{"line with a non-numeric uid is ignored", 5, "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := users.Name(tt.uid); got != tt.want {
				t.Fatalf("Name(%d) = %q, want %q", tt.uid, got, tt.want)
			}
		})
	}
}

func TestUsersMissingFileFallsBackToNumbers(t *testing.T) {
	users := NewUsers(filepath.Join(t.TempDir(), "does-not-exist"))
	if got := users.Name(0); got != "0" {
		t.Fatalf("Name(0) = %q, want %q", got, "0")
	}
}

func TestUsersAreCached(t *testing.T) {
	path := writePasswd(t, passwdFixture)
	users := NewUsers(path)
	if got := users.Name(0); got != "root" {
		t.Fatalf("Name(0) = %q, want root", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := users.Name(1000); got != "eliab" {
		t.Fatalf("after the file was removed Name(1000) = %q, want the cached eliab", got)
	}
}
