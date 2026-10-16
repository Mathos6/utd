package utd

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// overlayCandidates is the list of top level directories we overlay when they
// are present both in a package tree and on the system. On merged-/usr systems
// bin, sbin, lib and lib64 are symbolic links to their /usr counterpart, so
// overlaying /usr already covers them.
var overlayCandidates = []string{"usr", "opt", "lib", "lib64", "bin", "sbin", "libexec", "etc", "var"}

// Sandbox runs a command with the extracted package trees overlaid on the
// system directories using bubblewrap. This makes commands that look for their
// data files through hard coded paths (typically /usr/share/<name>) work, which
// a plain PATH based approach cannot do. It is entirely optional: when
// bubblewrap is absent or unusable, utd falls back to PATH and LD_LIBRARY_PATH
// injection.
type Sandbox struct {
	bwrap  string
	log    *Logger
	once   sync.Once
	usable bool
}

// NewSandbox returns a sandbox backed by bubblewrap if it is installed.
func NewSandbox(log *Logger) *Sandbox {
	s := &Sandbox{log: log}
	if p, err := exec.LookPath("bwrap"); err == nil {
		s.bwrap = p
	}
	return s
}

// Available reports whether the sandbox can be used. The capability probe is
// performed only once.
func (s *Sandbox) Available() bool {
	if s.bwrap == "" {
		return false
	}
	s.once.Do(func() {
		cmd := exec.Command(s.bwrap,
			"--dev-bind", "/", "/",
			"--overlay-src", "/usr", "--overlay-src", "/etc", "--ro-overlay", "/usr",
			"--", "true")
		cmd.Env = os.Environ()
		s.usable = cmd.Run() == nil
		if !s.usable {
			s.log.Verbosef("bubblewrap is installed but cannot create namespaces, falling back to PATH isolation")
		}
	})
	return s.usable
}

// Wrap returns the command and arguments that run bin inside the sandbox.
func (s *Sandbox) Wrap(cache *Cache, bin string, args []string) (string, []string) {
	wrapped := []string{"--dev-bind", "/", "/"}
	packages := cache.treeNames()
	for _, root := range overlayCandidates {
		sys := filepath.Join("/", root)
		if !isRealDir(sys) {
			continue
		}
		var srcs []string
		for _, pkg := range packages {
			src := filepath.Join(cache.TreeDir(), pkg, root)
			if dirExists(src) {
				srcs = append(srcs, src)
			}
		}
		if len(srcs) == 0 {
			continue
		}
		// Package content is listed first so it takes precedence, then the
		// system directory as the lowest (and read only) layer.
		for _, src := range srcs {
			wrapped = append(wrapped, "--overlay-src", src)
		}
		// --ro-overlay expects at least two layers.
		wrapped = append(wrapped, "--overlay-src", sys, "--ro-overlay", sys)
	}
	wrapped = append(wrapped, "--", bin)
	wrapped = append(wrapped, args...)
	return s.bwrap, wrapped
}

// isRealDir reports whether path is a directory that is not a symbolic link.
func isRealDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}
