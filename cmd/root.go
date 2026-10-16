package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mathieu/utd/internal/utd"
	"github.com/spf13/cobra"
)

// Version is the version string reported by `utd version`. It is set by main,
// which receives it from the build through -ldflags.
var Version = "dev"

const longHelp = `utd: Use Then Delete.

utd runs a program that is not installed on this system. If the command is
missing, utd asks the package manager (apt on Debian and derivatives) for the
package that provides it, downloads the package and its missing dependencies
into a temporary directory, extracts them there and runs the program with that
private tree in front of PATH. Nothing is installed system wide.

Downloaded files live in a cache directory (/tmp/utd by default) which the
system empties on reboot, so they disappear by themselves. With --delete=now
they are removed as soon as the command exits successfully.

When bubblewrap is available, the extracted files are additionally overlaid on
the system directories inside a private namespace, so that programs looking for
their data files through hard coded paths (for instance /usr/share/figlet) find
them. Use --no-sandbox to disable this.

Usage:
  utd [flags] <command> [args...]
  utd run [flags] <command> [args...]
  utd <subcommand>

The flags must appear before the command name. Everything after the command
name is passed to it untouched.

Examples:
  utd ffmpeg -i input.mp4 output.avi
  utd --delete=now sl
  utd --package=ripgrep rg --version
  utd --no-deps figlet hello
  utd search bat
  utd list

Flags:
  -d, --delete <when>    when to delete the downloaded packages:
                           now    remove them after a successful run
                           reboot remove them on the next reboot (default)
                           never  keep them in a persistent cache
  -p, --package <name>   use this package instead of guessing it
      --binary <name>    run this executable from the package
      --dir <path>       cache directory (default $UTD_DIR or /tmp/utd)
  -f, --force-download   download even if the command already exists locally
      --no-deps          do not download the dependencies
      --no-sandbox       do not use bubblewrap to overlay the extracted files
  -n, --dry-run          print what would be done, run nothing
  -v, --verbose          explain every step
  -q, --quiet            only report errors
      --no-color         disable colored output
  -V, --version          print the version
  -h, --help             print this help

Environment:
  UTD_DIR     overrides the default cache directory
  NO_COLOR    disables colored output when set
`

// NewRootCmd builds the utd command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "utd [flags] <command> [args...]",
		Short: "Use Then Delete: run a command without installing it",
		Long:  longHelp,
		// utd passes everything after the command name to the command, so
		// cobra must not try to interpret those as its own flags.
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithArgs(cmd, args)
		},
	}
	root.AddCommand(
		newRunCmd(),
		newCleanCmd(),
		newListCmd(),
		newSearchCmd(),
		newVersionCmd(),
	)
	return root
}

// runWithArgs parses the utd flags and runs the wrapped command.
func runWithArgs(cmd *cobra.Command, args []string) error {
	opts := utd.NewOptions()
	name, cmdArgs, err := parseArgs(args, opts)
	if err != nil {
		return err
	}
	if opts.Help {
		return cmd.Help()
	}
	if opts.ShowVersion {
		printVersion(cmd.OutOrStdout())
		return nil
	}
	if name == "" {
		return cmd.Help()
	}
	if err := opts.Validate(); err != nil {
		return err
	}
	applyDirDefault(opts)

	log := newLogger(opts)
	runner := utd.NewRunner(opts, log)
	code, err := runner.Run(name, cmdArgs)
	if err != nil {
		log.Errorf("%v", err)
		os.Exit(1)
	}
	os.Exit(code)
	return nil
}

// parseArgs splits the argument list into utd options and the wrapped command.
// Value taking options accept both "--opt=value" and "--opt value" forms.
func parseArgs(args []string, opts *utd.Options) (string, []string, error) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			break
		}
		switch {
		case a == "-h" || a == "--help":
			opts.Help = true
			i++
		case a == "-V" || a == "--version":
			opts.ShowVersion = true
			i++
		case a == "-v" || a == "--verbose":
			opts.Verbose = true
			i++
		case a == "-q" || a == "--quiet":
			opts.Quiet = true
			i++
		case a == "-n" || a == "--dry-run":
			opts.DryRun = true
			i++
		case a == "-f" || a == "--force-download":
			opts.ForceDownload = true
			i++
		case a == "--no-deps":
			opts.NoDeps = true
			i++
		case a == "--no-sandbox":
			opts.NoSandbox = true
			i++
		case a == "--no-color":
			opts.NoColor = true
			i++
		case strings.HasPrefix(a, "--delete="):
			opts.Delete = utd.DeleteWhen(strings.TrimPrefix(a, "--delete="))
			i++
		case a == "-d" || a == "--delete":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a value (now, reboot or never)", a)
			}
			if !utd.ValidDelete(args[i+1]) {
				return "", nil, fmt.Errorf("invalid --delete value %q (want now, reboot or never)", args[i+1])
			}
			opts.Delete = utd.DeleteWhen(args[i+1])
			i += 2
		case strings.HasPrefix(a, "--package="):
			opts.Package = strings.TrimPrefix(a, "--package=")
			i++
		case a == "-p" || a == "--package":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a value", a)
			}
			opts.Package = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--binary="):
			opts.Binary = strings.TrimPrefix(a, "--binary=")
			i++
		case a == "--binary":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a value", a)
			}
			opts.Binary = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--dir="):
			opts.Dir = strings.TrimPrefix(a, "--dir=")
			i++
		case a == "--dir":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a value", a)
			}
			opts.Dir = args[i+1]
			i += 2
		default:
			return "", nil, fmt.Errorf("unknown option %q (see utd --help)", a)
		}
	}
	rest := args[i:]
	if len(rest) == 0 {
		return "", nil, nil
	}
	if len(rest) == 1 {
		return rest[0], nil, nil
	}
	return rest[0], rest[1:], nil
}

// applyDirDefault fills in the cache directory when the user did not choose one.
func applyDirDefault(opts *utd.Options) {
	if opts.Dir != "" {
		return
	}
	if env := os.Getenv("UTD_DIR"); env != "" {
		opts.Dir = env
		return
	}
	if opts.Delete == utd.DeleteNever {
		if dir, err := os.UserCacheDir(); err == nil {
			opts.Dir = filepath.Join(dir, "utd")
			return
		}
	}
	opts.Dir = utd.DefaultDir
}

// defaultCacheDir mirrors applyDirDefault for the read-only subcommands.
func defaultCacheDir() string {
	if env := os.Getenv("UTD_DIR"); env != "" {
		return env
	}
	if !dirExists(utd.DefaultDir) {
		if dir, err := os.UserCacheDir(); err == nil {
			if p := filepath.Join(dir, "utd"); dirExists(p) {
				return p
			}
		}
	}
	return utd.DefaultDir
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// newLogger builds the logger from the resolved options.
func newLogger(opts *utd.Options) *utd.Logger {
	color := !opts.NoColor && os.Getenv("NO_COLOR") == "" && isTerminal(os.Stderr)
	return utd.NewLogger(opts.Level(), os.Stderr, color)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "utd version %s\n", Version)
}
