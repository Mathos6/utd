package utd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProviderResult is a package that may provide a command.
type ProviderResult struct {
	Package string
	Source  string
}

// PackageManager abstracts the distribution specific operations UTD needs.
type PackageManager interface {
	// Name returns a short identifier such as "apt".
	Name() string
	// Available reports whether the package manager can be used here.
	Available() bool
	// PackageExists reports whether pkg is a known package.
	PackageExists(pkg string) bool
	// IsInstalled reports whether pkg is already installed on the system.
	IsInstalled(pkg string) bool
	// ResolveDependencies returns the packages that would be installed to
	// satisfy pkg (that is pkg itself and its missing dependencies).
	ResolveDependencies(pkg string) ([]string, error)
	// Download fetches the given packages into dir and returns the created
	// .deb paths.
	Download(pkgs []string, dir string) ([]string, error)
	// Extract extracts a .deb archive into dest.
	Extract(deb, dest string) error
	// CommandProviders returns candidate packages providing a command.
	CommandProviders(cmd string) []ProviderResult
}

// aptEnv forces a stable, English, locale independent output so that parsing
// keeps working whatever the user's language is.
func aptEnv() []string {
	return cleanEnv(map[string]string{"LC_ALL": "C", "LANG": "C", "LANGUAGE": "C"})
}

// APT implements PackageManager on top of the Debian apt tooling.
type APT struct {
	log *Logger
}

// NewAPT returns an apt based package manager.
func NewAPT(log *Logger) *APT { return &APT{log: log} }

// Name implements PackageManager.
func (a *APT) Name() string { return "apt" }

// Available implements PackageManager.
func (a *APT) Available() bool {
	for _, tool := range []string{"apt-get", "apt-cache", "dpkg-deb", "dpkg-query"} {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}
	return true
}

func (a *APT) run(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = aptEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// PackageExists implements PackageManager.
func (a *APT) PackageExists(pkg string) bool {
	if pkg == "" {
		return false
	}
	_, err := a.run("", "apt-cache", "show", pkg)
	return err == nil
}

// IsInstalled implements PackageManager.
func (a *APT) IsInstalled(pkg string) bool {
	if pkg == "" {
		return false
	}
	out, err := a.run("", "dpkg-query", "-W", "-f=${db:Status-Abbrev}", pkg)
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(out), "ii")
}

// ResolveDependencies implements PackageManager. It relies on apt's own
// dependency solver in simulation mode, which works without root privileges and
// lists exactly the packages that would have to be installed.
func (a *APT) ResolveDependencies(pkg string) ([]string, error) {
	out, err := a.run("", "apt-get", "-s", "-y", "--no-install-recommends", "install", pkg)
	if err != nil {
		return nil, fmt.Errorf("apt-get -s install %s: %s", pkg, lastLines(out))
	}
	var pkgs []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		pkgs = append(pkgs, name)
	}
	return pkgs, nil
}

// Download implements PackageManager. Packages are fetched one by one so that
// a single unavailable name cannot abort the whole download.
func (a *APT) Download(pkgs []string, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var created []string
	var firstErr error
	for _, pkg := range pkgs {
		out, err := a.run(dir, "apt-get", "download", pkg)
		if err != nil {
			a.log.Verbosef("apt-get download %s failed: %s", pkg, lastLines(out))
			if firstErr == nil {
				firstErr = fmt.Errorf("could not download %s: %s", pkg, lastLines(out))
			}
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, globEscape(pkg)+"_*.deb"))
		if len(matches) == 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("download of %s produced no .deb", pkg)
			}
			continue
		}
		created = append(created, matches...)
	}
	return created, firstErr
}

// Extract implements PackageManager.
func (a *APT) Extract(deb, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	out, err := a.run("", "dpkg-deb", "-x", deb, dest)
	if err != nil {
		return fmt.Errorf("dpkg-deb -x %s: %s", filepath.Base(deb), lastLines(out))
	}
	return nil
}

// CommandProviders implements PackageManager by chaining several lookup
// strategies, from the most accurate to the most generic.
func (a *APT) CommandProviders(cmd string) []ProviderResult {
	var out []ProviderResult
	seen := map[string]bool{}
	add := func(pkgs []string, source string) {
		for _, p := range pkgs {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, ProviderResult{Package: p, Source: source})
		}
	}
	add(a.contentsProviders(cmd), "apt Contents index")
	add(a.commandNotFoundProviders(cmd), "command-not-found database")
	add(a.aptFileProviders(cmd), "apt-file")
	add(a.nameSearchProviders(cmd), "apt-cache search")
	return out
}
