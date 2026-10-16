package utd

// Alias describes a known mapping between a command and the package that
// provides it, optionally with the real name of the binary inside the package.
type Alias struct {
	// Package is the Debian package name.
	Package string
	// Binary is the executable to run. Empty means "same as the command".
	Binary string
}

// aliases covers the cases that cannot be resolved automatically:
//
//   - commands installed through the alternatives system, which therefore do
//     not appear as a real file in the Contents index (nc, awk, cc, ...);
//   - packages whose binary is renamed to avoid a conflict (bat, fd, ...).
//
// Everything else is discovered automatically from the apt indexes.
var aliases = map[string]Alias{
	// Provided through update-alternatives, invisible in the Contents index.
	"nc":     {Package: "netcat-openbsd", Binary: "nc.openbsd"},
	"netcat": {Package: "netcat-openbsd", Binary: "nc.openbsd"},
	"awk":    {Package: "mawk", Binary: "mawk"},
	"cc":     {Package: "gcc", Binary: "gcc"},
	"c++":    {Package: "g++", Binary: "g++"},
	"vi":     {Package: "vim", Binary: "vim"},
	"editor": {Package: "vim", Binary: "vim"},
	"pager":  {Package: "less", Binary: "less"},
	"python": {Package: "python3", Binary: "python3"},

	// Binary renamed inside the package.
	"bat":     {Package: "bat", Binary: "batcat"},
	"batcat":  {Package: "bat", Binary: "batcat"},
	"fd":      {Package: "fd-find", Binary: "fdfind"},
	"fdfind":  {Package: "fd-find", Binary: "fdfind"},
	"ripgrep": {Package: "ripgrep", Binary: "rg"},
	"ag":      {Package: "silversearcher-ag", Binary: "ag"},
	"ctags":   {Package: "universal-ctags", Binary: "ctags"},

	// Packages whose name has nothing to do with the command.
	"7z":       {Package: "7zip", Binary: "7z"},
	"7za":      {Package: "7zip", Binary: "7za"},
	"dig":      {Package: "bind9-dnsutils", Binary: "dig"},
	"host":     {Package: "bind9-dnsutils", Binary: "host"},
	"nslookup": {Package: "bind9-dnsutils", Binary: "nslookup"},
	"ping":     {Package: "iputils-ping", Binary: "ping"},
	"hexdump":  {Package: "bsdextrautils", Binary: "hexdump"},
	"exiftool": {Package: "libimage-exiftool-perl", Binary: "exiftool"},
	"mtr":      {Package: "mtr-tiny", Binary: "mtr"},
	"xxd":      {Package: "xxd", Binary: "xxd"},
}

// LookupAlias returns the alias registered for a command.
func LookupAlias(cmd string) (Alias, bool) {
	a, ok := aliases[cmd]
	return a, ok
}

// Aliases returns a copy of the alias table, for the search command.
func Aliases() map[string]Alias {
	out := make(map[string]Alias, len(aliases))
	for k, v := range aliases {
		out[k] = v
	}
	return out
}
