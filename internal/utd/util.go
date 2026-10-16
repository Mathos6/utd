package utd

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// isExecutableFile reports whether path is a regular file with an executable
// bit set.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// dirHasEntries reports whether dir contains at least one entry.
func dirHasEntries(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	return err == nil && len(names) > 0
}

// normalize reduces a name to its lower case letters and digits, so that
// "fd-find", "fdfind" and "fd_find" compare equal.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lastLines returns the last few non empty lines of s, trimmed. It is used to
// build readable error messages out of command output.
func lastLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	const max = 3
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	var out []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return "no output"
	}
	return strings.Join(out, "; ")
}

// cleanEnv returns the current environment with the given variables overridden.
// It removes any pre-existing occurrence of the keys first so that getenv finds
// the new value.
func cleanEnv(overrides map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if _, ok := overrides[key]; ok {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	return env
}

// prependPath returns env with dirs prepended to the value of key.
func prependPath(env []string, key string, dirs []string) []string {
	if len(dirs) == 0 {
		return env
	}
	joined := strings.Join(dirs, string(os.PathListSeparator))
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			old := kv[len(key)+1:]
			if old == "" {
				out = append(out, key+"="+joined)
			} else {
				out = append(out, key+"="+joined+string(os.PathListSeparator)+old)
			}
			found = true
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, key+"="+joined)
	}
	return out
}

// setEnv sets key to value in env, replacing any existing entry.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			if !replaced {
				out = append(out, key+"="+value)
				replaced = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, key+"="+value)
	}
	return out
}

// dirSize returns the total size in bytes of the regular files under dir.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// HumanSize is the exported form of humanSize, for the command layer.
func HumanSize(n int64) string { return humanSize(n) }

// DirectorySize returns the size in bytes of the regular files under dir.
func DirectorySize(dir string) int64 { return dirSize(dir) }

// humanSize formats a byte count in a compact, human friendly way.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return itoa(n) + " B"
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := float64(n)
	for _, u := range units {
		value /= unit
		if value < unit {
			return trimZero(value) + " " + u
		}
	}
	return trimZero(value) + " PiB"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func trimZero(f float64) string {
	s := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(f, 'f', 1, 64), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

// envInfo caches the architecture information of the running system.
type envInfo struct {
	Multiarch string // e.g. x86_64-linux-gnu
	DebArch   string // e.g. amd64
}

var (
	envInfoOnce sync.Once
	envInfoVal  envInfo
)

// Environ returns the (cached) architecture information for this machine.
func Environ() envInfo {
	envInfoOnce.Do(func() {
		envInfoVal = detectEnvInfo()
	})
	return envInfoVal
}

func detectEnvInfo() envInfo {
	var info envInfo
	if out, err := runQuiet("dpkg-architecture", "-qDEB_HOST_MULTIARCH"); err == nil {
		info.Multiarch = strings.TrimSpace(out)
	}
	if out, err := runQuiet("dpkg-architecture", "-qDEB_HOST_ARCH"); err == nil {
		info.DebArch = strings.TrimSpace(out)
	}
	return info
}

// runQuiet runs a command and returns its combined output.
func runQuiet(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
