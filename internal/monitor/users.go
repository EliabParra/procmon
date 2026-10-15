package monitor

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Users maps UIDs to user names by reading a passwd file once and caching it.
// It parses the file itself instead of using os/user.
type Users struct {
	path string

	once  sync.Once
	names map[int]string
}

// NewUsers returns a resolver for the passwd file at path (normally /etc/passwd).
func NewUsers(path string) *Users { return &Users{path: path} }

// Name returns the user name of uid, or the number itself when it is unknown
// or the file cannot be read.
func (u *Users) Name(uid int) string {
	u.once.Do(func() { u.names = loadPasswd(u.path) })
	if name, ok := u.names[uid]; ok {
		return name
	}
	return strconv.Itoa(uid)
}

// loadPasswd reads "name:password:uid:gid:..." lines. Comments, blank and
// malformed lines are skipped; when a UID repeats, the first entry wins, as
// getpwuid does. An unreadable file gives an empty map.
func loadPasswd(path string) map[int]string {
	names := make(map[int]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return names
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 || fields[0] == "" {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		if _, seen := names[uid]; !seen {
			names[uid] = fields[0]
		}
	}
	return names
}
