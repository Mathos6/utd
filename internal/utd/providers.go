package utd

import (
	"os/exec"
	"regexp"
	"strings"
)

// commandNotFoundProviders queries the database maintained by the
// command-not-found package (a small SQLite file). The database maps commands
// to the packages that ship them, which is exactly what we need. Reading it
// requires a SQLite engine: we use python3, which is present on every Debian
// and Ubuntu system that ships this database.
func (a *APT) commandNotFoundProviders(cmd string) []string {
	const db = "/var/lib/command-not-found/commands.db"
	if !fileExists(db) {
		return nil
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		return nil
	}
	const script = `import sqlite3,sys
con=sqlite3.connect("file:"+sys.argv[1]+"?mode=ro",uri=True)
q="SELECT p.name FROM commands c JOIN packages p ON c.pkgID=p.pkgID WHERE c.command=? ORDER BY p.priority,p.name"
for (name,) in con.execute(q,(sys.argv[2],)):
    print(name)`
	env := cleanEnv(map[string]string{"LC_ALL": "C"})
	raw, err := runWithEnv("", env, python, "-c", script, db, cmd)
	if err != nil {
		a.log.Verbosef("command-not-found lookup failed: %s", lastLines(raw))
		return nil
	}
	return nonEmptyLines(raw)
}

// aptFileProviders uses apt-file when its contents cache has already been
// built. We never trigger an apt-file update ourselves: that would download a
// large index and defeat the purpose of a throwaway tool.
func (a *APT) aptFileProviders(cmd string) []string {
	if !dirHasEntries("/var/cache/apt-file") {
		return nil
	}
	aptFile, err := exec.LookPath("apt-file")
	if err != nil {
		return nil
	}
	re := `(^|/)` + regexp.QuoteMeta(cmd) + `$`
	raw, err := runWithEnv("", aptEnv(), aptFile, "search", "--regexp", re)
	if err != nil {
		a.log.Verbosef("apt-file search failed: %s", lastLines(raw))
		return nil
	}
	var out []string
	for _, line := range nonEmptyLines(raw) {
		if i := strings.Index(line, ":"); i > 0 {
			out = append(out, strings.TrimSpace(line[:i]))
		}
	}
	return out
}

// nameSearchProviders is the last resort: a package whose name matches the
// command. It is cheap and often right, but it cannot find renamed packages,
// hence its low priority.
func (a *APT) nameSearchProviders(cmd string) []string {
	raw, err := a.run("", "apt-cache", "search", "--names-only", "^"+regexp.QuoteMeta(cmd)+"$")
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range nonEmptyLines(raw) {
		if i := strings.Index(line, " - "); i > 0 {
			out = append(out, strings.TrimSpace(line[:i]))
		}
	}
	return out
}

// runWithEnv runs a command with an explicit environment.
func runWithEnv(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
