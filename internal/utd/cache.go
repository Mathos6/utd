package utd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackageEntry describes one package present in the cache.
type PackageEntry struct {
	Name   string // package name (figlet)
	Dir    string // extracted tree
	Deb    string // .deb file, empty if it was already removed
	Size   int64  // size on disk of the extracted tree
	HasDeb bool
}

// Cache manages the on-disk layout of utd:
//
//	<root>/packages/<name>_<version>_<arch>.deb   downloaded archives
//	<root>/tree/<name>/...                        extracted contents
//	<root>/index.json                             command -> package mappings
type Cache struct {
	root string
	log  *Logger
}

// IndexEntry records how a command was resolved so the next invocation can
// reuse the already extracted package.
type IndexEntry struct {
	Package  string   `json:"package"`
	Binary   string   `json:"binary"`
	Packages []string `json:"packages,omitempty"`
}

// Index is the persistent command -> package mapping.
type Index struct {
	Commands map[string]IndexEntry `json:"commands"`
}

// NewCache returns a cache rooted at root.
func NewCache(root string, log *Logger) *Cache {
	return &Cache{root: root, log: log}
}

// Root returns the cache root directory.
func (c *Cache) Root() string { return c.root }

// DebDir returns the directory holding the downloaded .deb files.
func (c *Cache) DebDir() string { return filepath.Join(c.root, "packages") }

// TreeDir returns the directory holding the extracted package trees.
func (c *Cache) TreeDir() string { return filepath.Join(c.root, "tree") }

// ExtractedDir returns the extraction directory of a package.
func (c *Cache) ExtractedDir(pkg string) string { return filepath.Join(c.TreeDir(), pkg) }

func (c *Cache) indexFile() string { return filepath.Join(c.root, "index.json") }

// Ensure creates the cache directories.
func (c *Cache) Ensure() error {
	if err := os.MkdirAll(c.DebDir(), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(c.TreeDir(), 0o755)
}

// HasExtracted reports whether the package has already been extracted.
func (c *Cache) HasExtracted(pkg string) bool {
	return dirExists(c.ExtractedDir(pkg))
}

// FindDeb returns the path of the .deb of a package if it is already cached.
func (c *Cache) FindDeb(pkg string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(c.DebDir(), globEscape(pkg)+"_*.deb"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	sort.Strings(matches)
	return matches[0], true
}

// GlobPkgDeb is like FindDeb but exported for package managers that need to
// locate freshly downloaded archives.
func (c *Cache) GlobPkgDeb(pkg string) []string {
	matches, _ := filepath.Glob(filepath.Join(c.DebDir(), globEscape(pkg)+"_*.deb"))
	sort.Strings(matches)
	return matches
}

// ListPackages returns the packages currently extracted, sorted by name.
func (c *Cache) ListPackages() []PackageEntry {
	entries, err := os.ReadDir(c.TreeDir())
	if err != nil {
		return nil
	}
	var out []PackageEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pe := PackageEntry{Name: e.Name(), Dir: c.ExtractedDir(e.Name())}
		if deb, ok := c.FindDeb(e.Name()); ok {
			pe.Deb = deb
			pe.HasDeb = true
		}
		out = append(out, pe)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RemovePackage deletes the extracted tree and the archives of a package.
func (c *Cache) RemovePackage(pkg string) error {
	var firstErr error
	if err := os.RemoveAll(c.ExtractedDir(pkg)); err != nil {
		firstErr = err
	}
	for _, deb := range c.GlobPkgDeb(pkg) {
		if err := os.Remove(deb); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := c.forgetCommand(pkg); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// RemoveAll deletes the whole cache directory.
func (c *Cache) RemoveAll() error {
	root := filepath.Clean(c.root)
	if root == "" || root == "/" || root == "." || root == os.Getenv("HOME") {
		return fmt.Errorf("refusing to remove unsafe cache directory %q", c.root)
	}
	return os.RemoveAll(root)
}

// IndexLoad reads the persistent index, returning an empty one when absent.
func (c *Cache) IndexLoad() *Index {
	idx := &Index{Commands: map[string]IndexEntry{}}
	data, err := os.ReadFile(c.indexFile())
	if err != nil {
		return idx
	}
	if err := json.Unmarshal(data, idx); err != nil {
		c.log.Verbosef("ignoring corrupt index %s: %v", c.indexFile(), err)
		return &Index{Commands: map[string]IndexEntry{}}
	}
	if idx.Commands == nil {
		idx.Commands = map[string]IndexEntry{}
	}
	return idx
}

// IndexSave writes the index back to disk.
func (c *Cache) IndexSave(idx *Index) error {
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.indexFile() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.indexFile())
}

// Remember records that command maps to package/binary.
func (c *Cache) Remember(command, pkg, binary string, packages []string) {
	idx := c.IndexLoad()
	idx.Commands[command] = IndexEntry{Package: pkg, Binary: binary, Packages: packages}
	_ = c.IndexSave(idx)
}

// forgetCommand removes every index entry that points at pkg.
func (c *Cache) forgetCommand(pkg string) error {
	idx := c.IndexLoad()
	changed := false
	for cmd, e := range idx.Commands {
		if e.Package == pkg || c.binaryGone(e.Binary) {
			delete(idx.Commands, cmd)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return c.IndexSave(idx)
}

func (c *Cache) binaryGone(binary string) bool {
	if binary == "" {
		return false
	}
	_, ok := c.FindBinary(binary)
	return !ok
}

// binarySubdirs lists the directories inside a package tree where executables
// may live, ordered by priority.
var binarySubdirs = []string{
	"usr/bin", "usr/sbin", "bin", "sbin",
	"usr/local/bin", "usr/local/sbin", "usr/games",
}

// FindBinary looks for an executable named name across all extracted packages.
func (c *Cache) FindBinary(name string) (string, bool) {
	entries, err := os.ReadDir(c.TreeDir())
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, sub := range binarySubdirs {
			p := filepath.Join(c.TreeDir(), e.Name(), sub, name)
			if isExecutableFile(p) {
				return p, true
			}
		}
	}
	return "", false
}

// FindPackageBinary looks for an executable named name inside one package only.
func (c *Cache) FindPackageBinary(pkg, name string) (string, bool) {
	for _, sub := range binarySubdirs {
		p := filepath.Join(c.ExtractedDir(pkg), sub, name)
		if isExecutableFile(p) {
			return p, true
		}
	}
	return "", false
}

// FindFuzzyBinary tries to locate the binary of a package when its name does
// not match the command exactly (e.g. command "fd" -> binary "fdfind").
func (c *Cache) FindFuzzyBinary(pkg, command string) (string, bool) {
	target := normalize(command)
	if target == "" {
		return "", false
	}
	best := ""
	for _, sub := range binarySubdirs {
		dir := filepath.Join(c.ExtractedDir(pkg), sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if !isExecutableFile(p) {
				continue
			}
			n := normalize(e.Name())
			switch {
			case n == target:
				return p, true
			case best == "" && (strings.Contains(n, target) || strings.Contains(target, n)):
				best = p
			}
		}
	}
	return best, best != ""
}

// BinDirs returns every existing binary directory inside the cache, with the
// directories of the package that was cached last first.
func (c *Cache) BinDirs() []string {
	var out []string
	for _, pkg := range c.treeNames() {
		for _, sub := range binarySubdirs {
			dir := filepath.Join(c.TreeDir(), pkg, sub)
			if dirExists(dir) {
				out = append(out, dir)
			}
		}
	}
	return dedupe(out)
}

// LibDirs returns every existing library directory inside the cache.
func (c *Cache) LibDirs() []string {
	multi := Environ().Multiarch
	subs := []string{}
	if multi != "" {
		subs = append(subs,
			filepath.Join("usr", "lib", multi),
			filepath.Join("lib", multi),
		)
	}
	subs = append(subs,
		filepath.Join("usr", "lib64"), "lib64",
		filepath.Join("usr", "lib"), "lib",
		filepath.Join("usr", "local", "lib"),
	)
	var out []string
	for _, pkg := range c.treeNames() {
		for _, sub := range subs {
			dir := filepath.Join(c.TreeDir(), pkg, sub)
			if dirExists(dir) {
				out = append(out, dir)
			}
		}
	}
	return dedupe(out)
}

// DataDirs returns the share directories inside the cache.
func (c *Cache) DataDirs() []string {
	var out []string
	for _, pkg := range c.treeNames() {
		for _, sub := range []string{filepath.Join("usr", "share"), filepath.Join("usr", "local", "share")} {
			dir := filepath.Join(c.TreeDir(), pkg, sub)
			if dirExists(dir) {
				out = append(out, dir)
			}
		}
	}
	return dedupe(out)
}

// Env builds the environment used to run a cached command: the cache binary
// and library directories are prepended so the program finds its own files
// before the system ones.
func (c *Cache) Env() []string {
	env := os.Environ()
	env = prependPath(env, "PATH", c.BinDirs())
	env = prependPath(env, "LD_LIBRARY_PATH", c.LibDirs())
	env = prependPath(env, "XDG_DATA_DIRS", c.DataDirs())
	env = setEnv(env, "UTD_ROOT", c.root)
	env = setEnv(env, "UTD=1", "1")
	return env
}

func (c *Cache) treeNames() []string {
	entries, err := os.ReadDir(c.TreeDir())
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// globEscape escapes the characters of a package name that are special in
// filepath.Glob patterns.
func globEscape(pkg string) string {
	var b strings.Builder
	for _, r := range pkg {
		if strings.ContainsRune(`*?[\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
