package utd

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pierrec/lz4/v4"
)

// aptListsDir is where apt keeps its package indexes.
const aptListsDir = "/var/lib/apt/lists"

// contentsProviders looks for the packages shipping a command using the apt
// Contents indexes. Those indexes map every file of every package to its
// package, which makes them the most accurate way to answer "which package
// provides bin/foo?". They are optional: not every system has them.
func (a *APT) contentsProviders(cmd string) []string {
	files := contentsFiles()
	if len(files) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		for _, pkg := range searchContentsFile(f, cmd) {
			if !seen[pkg] {
				seen[pkg] = true
				out = append(out, pkg)
			}
		}
		a.log.Verbosef("Contents %s: %d provider(s) for %q so far", filepath.Base(f), len(out), cmd)
	}
	return out
}

// contentsFiles returns the Contents indexes to scan, architecture specific
// first, then the architecture independent one, then the rest.
func contentsFiles() []string {
	entries, err := os.ReadDir(aptListsDir)
	if err != nil {
		return nil
	}
	arch := "Contents-" + Environ().DebArch + "."
	var specific, all, other []string
	for _, e := range entries {
		name := e.Name()
		i := strings.Index(name, "Contents-")
		if i < 0 {
			continue
		}
		base := name[i:]
		if !strings.HasSuffix(base, ".lz4") && !strings.HasSuffix(base, ".gz") {
			continue
		}
		full := filepath.Join(aptListsDir, name)
		switch {
		case strings.HasPrefix(base, arch):
			specific = append(specific, full)
		case strings.HasPrefix(base, "Contents-all."):
			all = append(all, full)
		default:
			other = append(other, full)
		}
	}
	sort.Strings(specific)
	sort.Strings(all)
	sort.Strings(other)
	// Foreign architectures cannot provide binaries for this machine, so they
	// are only considered when we could not determine the local architecture.
	if Environ().DebArch == "" {
		return append(append(specific, all...), other...)
	}
	return append(specific, all...)
}

// searchContentsFile scans one Contents index for the exact paths where cmd
// could live and returns the packages that own them. The index is sorted by
// path, so the scan stops as soon as it is past the last possible match.
func searchContentsFile(path, cmd string) []string {
	targets := map[string]bool{}
	maxTarget := ""
	for _, dir := range []string{"bin", "sbin", "usr/bin", "usr/sbin", "usr/games"} {
		t := dir + "/" + cmd
		targets[t] = true
		if t > maxTarget {
			maxTarget = t
		}
	}

	rc, err := openContents(path)
	if err != nil {
		return nil
	}
	defer rc.Close()

	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out []string
	seen := map[string]bool{}
	for sc.Scan() {
		line := sc.Text()
		sp := strings.IndexAny(line, " \t")
		if sp < 0 {
			continue
		}
		p := line[:sp]
		if p > maxTarget {
			break
		}
		if !targets[p] {
			continue
		}
		for _, item := range strings.Split(strings.TrimSpace(line[sp:]), ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			// Drop the leading "<area>/" (e.g. "utils/ripgrep").
			if i := strings.IndexByte(item, '/'); i >= 0 {
				item = item[i+1:]
			}
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}

// openContents opens a possibly compressed Contents index.
func openContents(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasSuffix(path, ".lz4"):
		return &multiCloser{Reader: lz4.NewReader(f), closers: []io.Closer{f}}, nil
	case strings.HasSuffix(path, ".gz"):
		zr, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &multiCloser{Reader: zr, closers: []io.Closer{zr, f}}, nil
	case strings.HasSuffix(path, ".xz"):
		f.Close()
		return nil, fmt.Errorf("xz compressed Contents indexes are not supported")
	default:
		return f, nil
	}
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
