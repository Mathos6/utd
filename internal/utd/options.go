package utd

import "fmt"

// DeleteWhen describes when the packages downloaded for a run are removed.
type DeleteWhen string

const (
	// DeleteNow removes the packages as soon as the command exits successfully.
	DeleteNow DeleteWhen = "now"
	// DeleteReboot leaves the packages in the cache directory. The default
	// cache lives under /tmp, which is wiped on reboot, so they disappear by
	// themselves.
	DeleteReboot DeleteWhen = "reboot"
	// DeleteNever keeps the packages around across reboots by using a
	// persistent cache directory.
	DeleteNever DeleteWhen = "never"
)

// DefaultDir is where packages are downloaded and extracted by default. It is
// deliberately under /tmp so that the system cleans it up on reboot.
const DefaultDir = "/tmp/utd"

// Options holds the settings resolved from the command line.
type Options struct {
	// Delete selects when downloaded packages are removed.
	Delete DeleteWhen
	// Package overrides the automatically resolved package name.
	Package string
	// Binary overrides the binary name to look for inside the package.
	Binary string
	// Dir is the cache directory.
	Dir string
	// Verbose enables verbose logging.
	Verbose bool
	// Quiet only keeps errors.
	Quiet bool
	// NoDeps disables the automatic download of dependencies.
	NoDeps bool
	// ForceDownload downloads the command even if it already exists locally.
	ForceDownload bool
	// DryRun only prints what would be done.
	DryRun bool
	// NoSandbox disables the bubblewrap based overlay even when available.
	NoSandbox bool
	// NoColor disables colored output.
	NoColor bool
	// Help requests the usage message.
	Help bool
	// ShowVersion requests the version.
	ShowVersion bool
}

// ValidDelete reports whether s is a supported --delete value.
func ValidDelete(s string) bool {
	switch DeleteWhen(s) {
	case DeleteNow, DeleteReboot, DeleteNever:
		return true
	}
	return false
}

// NewOptions returns options populated with their default values.
func NewOptions() *Options {
	return &Options{Delete: DeleteReboot}
}

// Validate checks the consistency of the options.
func (o *Options) Validate() error {
	if !ValidDelete(string(o.Delete)) {
		return fmt.Errorf("invalid --delete value %q (want now, reboot or never)", o.Delete)
	}
	if o.Quiet && o.Verbose {
		return fmt.Errorf("--quiet and --verbose are mutually exclusive")
	}
	return nil
}

// Level maps the options to a logger level.
func (o *Options) Level() Level {
	switch {
	case o.Quiet:
		return LevelQuiet
	case o.Verbose:
		return LevelVerbose
	default:
		return LevelNormal
	}
}
