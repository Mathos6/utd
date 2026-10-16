package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/mathieu/utd/internal/utd"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var dir string
	list := &cobra.Command{
		Use:           "list",
		Short:         "List the packages currently downloaded in the cache",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = defaultCacheDir()
			}
			out := cmd.OutOrStdout()
			log := utd.NewLogger(utd.LevelQuiet, io.Discard, false)
			cache := utd.NewCache(dir, log)

			pkgs := cache.ListPackages()
			if len(pkgs) == 0 {
				fmt.Fprintf(out, "cache %s is empty\n", dir)
				return nil
			}

			idx := cache.IndexLoad()
			byPkg := map[string][]string{}
			for name, entry := range idx.Commands {
				byPkg[entry.Package] = append(byPkg[entry.Package], name)
			}

			w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "PACKAGE\tSIZE\tARCHIVE\tCOMMANDS")
			var total int64
			for _, p := range pkgs {
				size := utd.DirectorySize(p.Dir)
				total += size
				archive := "-"
				if p.HasDeb {
					archive = filepath.Base(p.Deb)
				}
				cmds := byPkg[p.Name]
				sort.Strings(cmds)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, utd.HumanSize(size), archive, join(cmds))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d package(s), %s in %s\n", len(pkgs), utd.HumanSize(total), dir)
			return nil
		},
	}
	list.Flags().StringVar(&dir, "dir", "", "cache directory")
	return list
}

func join(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
