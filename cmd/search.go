package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/mathieu/utd/internal/utd"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var (
		dir     string
		verbose bool
	)
	search := &cobra.Command{
		Use:           "search <command>",
		Short:         "Show which packages can provide a command",
		Long:          "Search resolves a command to the packages that may provide it, using the same logic as a normal run, and prints the candidates in the order they would be tried.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = defaultCacheDir()
			}
			opts := utd.NewOptions()
			opts.Dir = dir
			opts.Verbose = verbose
			log := utd.NewLogger(opts.Level(), cmd.ErrOrStderr(), false)
			res := utd.NewResolver(utd.NewAPT(log), log)

			cands := res.Candidates(args[0])
			if len(cands) == 0 {
				return fmt.Errorf("no package seems to provide %q", args[0])
			}
			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SOURCE\tPACKAGE\tBINARY")
			for _, c := range cands {
				binary := c.Binary
				if binary == "" {
					binary = args[0]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", c.Source, c.Package, binary)
			}
			return w.Flush()
		},
	}
	search.Flags().StringVar(&dir, "dir", "", "cache directory")
	search.Flags().BoolVarP(&verbose, "verbose", "v", false, "explain every step")
	return search
}
