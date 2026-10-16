package utd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Runner orchestrates the whole "use then delete" flow.
type Runner struct {
	opts       *Options
	log        *Logger
	cache      *Cache
	mgr        PackageManager
	res        *Resolver
	sandbox    *Sandbox
	used       map[string]bool // packages used by the current run
	downloaded map[string]bool // packages fetched during the current run
}

// NewRunner builds a runner for the given options.
func NewRunner(opts *Options, log *Logger) *Runner {
	cache := NewCache(opts.Dir, log)
	var mgr PackageManager = NewAPT(log)
	return &Runner{
		opts:       opts,
		log:        log,
		cache:      cache,
		mgr:        mgr,
		res:        NewResolver(mgr, log),
		sandbox:    NewSandbox(log),
		used:       map[string]bool{},
		downloaded: map[string]bool{},
	}
}

// Cache exposes the cache (used by the clean and list subcommands).
func (r *Runner) Cache() *Cache { return r.cache }

// Resolver exposes the command resolver (used by the search subcommand).
func (r *Runner) Resolver() *Resolver { return r.res }

// Manager exposes the package manager.
func (r *Runner) Manager() PackageManager { return r.mgr }

// Run executes name with args, returning the exit code of the wrapped command.
func (r *Runner) Run(name string, args []string) (int, error) {
	if strings.TrimSpace(name) == "" {
		return 2, fmt.Errorf("no command given")
	}

	// Paths and relative files are executed as-is, they are never downloaded.
	if strings.ContainsRune(name, os.PathSeparator) || strings.HasPrefix(name, ".") {
		return r.exec(name, args, nil)
	}

	// The command already exists on the system: run it, unless the user
	// explicitly asked for a package or forced a download.
	if r.opts.Package == "" && r.opts.Binary == "" && !r.opts.ForceDownload {
		if path, err := exec.LookPath(name); err == nil {
			r.log.Infof("%s is already installed, running it directly", name)
			r.log.Verbosef("resolved to %s", path)
			return r.exec(path, args, nil)
		}
	}

	if err := r.cache.Ensure(); err != nil {
		return 1, fmt.Errorf("preparing cache: %w", err)
	}

	// A previous run may already have downloaded this command.
	if r.opts.Package == "" {
		if bin, entry, ok := r.cachedBinary(name); ok {
			r.log.Verbosef("using cached %s (package %s)", name, entry.Package)
			code, err := r.execCached(bin, args)
			if err == nil && code == 0 && r.opts.Delete == DeleteNow {
				r.cleanup(entry.Packages)
			}
			return code, err
		}
	}

	cands := r.primaryCandidates(name)
	fromPrimary := len(cands) > 0
	if !fromPrimary {
		cands = r.extendedCandidates(name, nil)
	}
	if len(cands) == 0 {
		return 1, fmt.Errorf("no package seems to provide %q; retry with --package=<name>", name)
	}

	if r.opts.DryRun {
		r.printPlan(name, args, cands[0])
		return 0, nil
	}

	code, ok, err := r.tryCandidates(name, args, cands)
	if err != nil {
		return code, err
	}
	if ok {
		return code, nil
	}

	// The cheap candidates did not ship the command. Fall back to scanning the
	// package indexes, unless they are what we already tried.
	if fromPrimary {
		exclude := map[string]bool{}
		for _, c := range cands {
			exclude[c.Package] = true
		}
		if deep := r.extendedCandidates(name, exclude); len(deep) > 0 {
			code, ok, err = r.tryCandidates(name, args, deep)
			if err != nil {
				return code, err
			}
			if ok {
				return code, nil
			}
		}
	}
	return 1, fmt.Errorf("no package providing %q could be found; retry with --package=<name>", name)
}

// tryCandidates provisions and runs the first candidate that works. ok is false
// when every candidate was rejected because it does not ship the command.
func (r *Runner) tryCandidates(name string, args []string, cands []Candidate) (int, bool, error) {
	for _, c := range cands {
		bin, err := r.provision(name, c)
		if err != nil {
			r.log.Verbosef("candidate %s (%s) rejected: %v", c.Package, c.Source, err)
			continue
		}
		r.cache.Remember(name, c.Package, filepath.Base(bin), r.usedList())
		code, err := r.execCached(bin, args)
		if err != nil {
			return code, false, err
		}
		if r.opts.Delete == DeleteNow && code == 0 {
			r.cleanup(r.usedList())
		}
		return code, true, nil
	}
	return 0, false, nil
}

// cachedBinary returns an already downloaded executable for a command.
func (r *Runner) cachedBinary(name string) (string, IndexEntry, bool) {
	idx := r.cache.IndexLoad()
	entry, found := idx.Commands[name]
	if !found {
		return "", IndexEntry{}, false
	}
	if bin, ok := r.cache.FindBinary(entry.Binary); ok {
		return bin, entry, true
	}
	if bin, ok := r.cache.FindPackageBinary(entry.Package, name); ok {
		return bin, entry, true
	}
	return "", IndexEntry{}, false
}

// primaryCandidates is the cheap tier: --package, built-in aliases and a
// package named like the command. It needs no index scan.
func (r *Runner) primaryCandidates(name string) []Candidate {
	if r.opts.Package != "" {
		if !r.mgr.PackageExists(r.opts.Package) {
			r.log.Warnf("package %q was not found in the package index", r.opts.Package)
		}
		return []Candidate{{Package: r.opts.Package, Binary: r.opts.Binary, Source: "--package"}}
	}
	out := r.res.Primary(name)
	if r.opts.Binary != "" {
		for i := range out {
			out[i].Binary = r.opts.Binary
		}
	}
	return out
}

// extendedCandidates is the expensive tier: it scans the package indexes.
func (r *Runner) extendedCandidates(name string, exclude map[string]bool) []Candidate {
	if r.opts.Package != "" {
		return nil
	}
	var out []Candidate
	for _, c := range r.res.Extended(name) {
		if exclude[c.Package] {
			continue
		}
		if r.opts.Binary != "" {
			c.Binary = r.opts.Binary
		}
		out = append(out, c)
	}
	return out
}

// provision makes sure the candidate package is downloaded and extracted, and
// returns the path of the executable to run.
func (r *Runner) provision(name string, c Candidate) (string, error) {
	if err := r.ensurePackage(c.Package); err != nil {
		return "", err
	}
	bin := c.Binary
	if bin == "" {
		bin = name
	}
	if p, ok := r.cache.FindPackageBinary(c.Package, bin); ok {
		return p, nil
	}
	if c.Binary == "" {
		if p, ok := r.cache.FindBinary(name); ok {
			return p, nil
		}
		if p, ok := r.cache.FindFuzzyBinary(c.Package, name); ok {
			r.log.Verbosef("package %s does not ship %q, using %s instead", c.Package, name, filepath.Base(p))
			return p, nil
		}
	}
	return "", fmt.Errorf("package %s does not provide a %q executable", c.Package, bin)
}

// ensurePackage downloads and extracts a package and its missing dependencies.
func (r *Runner) ensurePackage(pkg string) error {
	if r.cache.HasExtracted(pkg) {
		r.used[pkg] = true
		return nil
	}

	pkgs := []string{pkg}
	if !r.opts.NoDeps {
		deps, err := r.mgr.ResolveDependencies(pkg)
		if err != nil {
			r.log.Warnf("could not resolve dependencies of %s: %v", pkg, err)
		}
		for _, d := range deps {
			if d == pkg || r.mgr.IsInstalled(d) {
				// Dependencies already present on the system are used
				// directly: no need to download a second copy.
				continue
			}
			if !r.cache.HasExtracted(d) {
				pkgs = append(pkgs, d)
			}
		}
	}
	pkgs = dedupe(pkgs)

	var toDownload []string
	for _, p := range pkgs {
		if _, ok := r.cache.FindDeb(p); !ok {
			toDownload = append(toDownload, p)
		}
	}
	if len(toDownload) > 0 {
		r.log.Infof("downloading %s", strings.Join(toDownload, ", "))
		created, err := r.mgr.Download(toDownload, r.cache.DebDir())
		for _, deb := range created {
			r.downloaded[debPackageName(deb)] = true
		}
		if err != nil {
			r.log.Warnf("%v", err)
		}
	}

	for _, p := range pkgs {
		r.used[p] = true
		if r.cache.HasExtracted(p) {
			continue
		}
		deb, ok := r.cache.FindDeb(p)
		if !ok {
			if p == pkg {
				return fmt.Errorf("could not obtain package %q", p)
			}
			r.log.Warnf("dependency %q is unavailable, continuing without it", p)
			delete(r.used, p)
			continue
		}
		r.log.Verbosef("extracting %s", filepath.Base(deb))
		if err := r.mgr.Extract(deb, r.cache.ExtractedDir(p)); err != nil {
			_ = os.RemoveAll(r.cache.ExtractedDir(p))
			delete(r.used, p)
			if p == pkg {
				return err
			}
			r.log.Warnf("could not extract dependency %q: %v", p, err)
		}
	}
	return nil
}

// cleanup removes the given packages from the cache.
func (r *Runner) cleanup(pkgs []string) {
	names := dedupe(append([]string{}, pkgs...))
	if len(names) == 0 {
		r.log.Verbosef("nothing to delete: nothing was downloaded for this command")
		return
	}
	sort.Strings(names)
	var deleted []string
	for _, p := range names {
		if !r.cache.HasExtracted(p) {
			if _, ok := r.cache.FindDeb(p); !ok {
				continue
			}
		}
		if err := r.cache.RemovePackage(p); err != nil {
			r.log.Warnf("could not delete %s: %v", p, err)
			continue
		}
		deleted = append(deleted, p)
	}
	if len(deleted) > 0 {
		r.log.Infof("deleted %s", strings.Join(deleted, ", "))
	}
}

func (r *Runner) usedList() []string {
	out := make([]string, 0, len(r.used))
	for p := range r.used {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// execCached runs a cached binary with the cache environment and, when
// available, inside the bubblewrap overlay sandbox.
func (r *Runner) execCached(bin string, args []string) (int, error) {
	env := r.cache.Env()
	command, commandArgs := bin, args
	if !r.opts.NoSandbox && r.sandbox != nil && r.sandbox.Available() {
		command, commandArgs = r.sandbox.Wrap(r.cache, bin, args)
		r.log.Verbosef("running inside a bubblewrap overlay")
	}
	return r.exec(command, commandArgs, env)
}

// exec runs a binary, wiring the standard streams and returning its exit code.
func (r *Runner) exec(bin string, args []string, env []string) (int, error) {
	if r.opts.DryRun {
		r.log.Infof("dry-run: %s", commandLine(bin, args))
		return 0, nil
	}
	c := exec.Command(bin, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if env != nil {
		c.Env = env
	}
	err := c.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code := ee.ExitCode()
		if code < 0 {
			// Killed by a signal: use the shell convention 128+signal.
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				code = 128 + int(ws.Signal())
			} else {
				code = 128
			}
		}
		return code, nil
	}
	return 1, err
}

func (r *Runner) printPlan(name string, args []string, c Candidate) {
	pkgs := []string{c.Package}
	if !r.opts.NoDeps {
		if deps, err := r.mgr.ResolveDependencies(c.Package); err == nil {
			for _, d := range deps {
				if d == c.Package || r.mgr.IsInstalled(d) {
					continue
				}
				pkgs = append(pkgs, d)
			}
		}
	}
	r.log.Infof("dry-run: command     : %s", name)
	r.log.Infof("dry-run: package     : %s (via %s)", c.Package, c.Source)
	r.log.Infof("dry-run: to download : %s", strings.Join(dedupe(pkgs), ", "))
	r.log.Infof("dry-run: cache       : %s", r.cache.Root())
	r.log.Infof("dry-run: isolation   : %s", r.isolation())
	r.log.Infof("dry-run: would run   : %s", commandLine(name, args))
}

// isolation names the mechanism used to run the cached command.
func (r *Runner) isolation() string {
	if r.opts.NoSandbox || r.sandbox == nil || !r.sandbox.Available() {
		return "PATH and LD_LIBRARY_PATH"
	}
	return "bubblewrap overlay (use --no-sandbox to disable)"
}

// commandLine renders a command for display.
func commandLine(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, bin)
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			parts = append(parts, fmt.Sprintf("%q", a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// debPackageName extracts the package name from a .deb file name.
func debPackageName(deb string) string {
	base := filepath.Base(deb)
	if i := strings.IndexByte(base, '_'); i > 0 {
		return base[:i]
	}
	return strings.TrimSuffix(base, ".deb")
}
