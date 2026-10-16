package utd

import "strings"

// Candidate is a package that may provide a command.
type Candidate struct {
	// Package is the Debian package name.
	Package string
	// Binary is the executable to run, when it differs from the command.
	Binary string
	// Source describes how the candidate was found, for diagnostics.
	Source string
}

// Resolver turns a command name into candidate packages.
type Resolver struct {
	mgr PackageManager
	log *Logger
}

// NewResolver returns a resolver backed by a package manager.
func NewResolver(mgr PackageManager, log *Logger) *Resolver {
	return &Resolver{mgr: mgr, log: log}
}

// candidateSet accumulates candidates while skipping duplicates.
type candidateSet struct {
	out  []Candidate
	seen map[string]bool
}

func newCandidateSet() *candidateSet {
	return &candidateSet{seen: map[string]bool{}}
}

func (s *candidateSet) add(pkg, binary, source string) {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" || s.seen[pkg] {
		return
	}
	s.seen[pkg] = true
	s.out = append(s.out, Candidate{Package: pkg, Binary: binary, Source: source})
}

// Primary returns the candidates that can be found without scanning the
// package indexes: the built-in aliases and a package that carries the same
// name as the command. This is cheap and answers almost every request.
func (r *Resolver) Primary(cmd string) []Candidate {
	set := newCandidateSet()
	if a, ok := LookupAlias(cmd); ok {
		set.add(a.Package, a.Binary, "utd alias")
	}
	if r.mgr.PackageExists(cmd) {
		set.add(cmd, "", "package of the same name")
	}
	return set.out
}

// Extended returns the candidates discovered by scanning the package indexes
// (apt Contents, command-not-found, apt-file, apt-cache). It is only worth
// calling when the primary candidates are insufficient.
func (r *Resolver) Extended(cmd string) []Candidate {
	set := newCandidateSet()
	for _, p := range r.mgr.CommandProviders(cmd) {
		set.add(p.Package, "", p.Source)
	}
	return set.out
}

// Candidates returns the full resolution order (primary then extended).
func (r *Resolver) Candidates(cmd string) []Candidate {
	set := newCandidateSet()
	for _, c := range r.Primary(cmd) {
		set.add(c.Package, c.Binary, c.Source)
	}
	for _, c := range r.Extended(cmd) {
		set.add(c.Package, c.Binary, c.Source)
	}
	return set.out
}

// Resolve returns the best candidate for cmd.
func (r *Resolver) Resolve(cmd string) (Candidate, bool) {
	c := r.Primary(cmd)
	if len(c) == 0 {
		c = r.Extended(cmd)
	}
	if len(c) == 0 {
		return Candidate{}, false
	}
	return c[0], true
}
