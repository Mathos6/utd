package cmd

import (
	"fmt"

	"github.com/mathieu/utd/internal/utd"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var (
		dir string
		all bool
	)
	clean := &cobra.Command{
		Use:           "clean [package...]",
		Short:         "Delete downloaded packages from the cache",
		Long:          "Clean removes downloaded packages from the cache. Without arguments it removes every package while keeping the cache directory. With arguments only the named packages are removed. --all removes the whole cache directory.",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = defaultCacheDir()
			}
			log := utd.NewLogger(utd.LevelNormal, cmd.OutOrStdout(), false)
			cache := utd.NewCache(dir, log)

			if all {
				if err := cache.RemoveAll(); err != nil {
					return fmt.Errorf("removing %s: %w", dir, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", dir)
				return nil
			}

			targets := args
			if len(targets) == 0 {
				for _, p := range cache.ListPackages() {
					targets = append(targets, p.Name)
				}
			}
			if len(targets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "cache is already empty")
				return nil
			}
			for _, pkg := range targets {
				if err := cache.RemovePackage(pkg); err != nil {
					return fmt.Errorf("removing %s: %w", pkg, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", pkg)
			}
			return nil
		},
	}
	clean.Flags().StringVar(&dir, "dir", "", "cache directory")
	clean.Flags().BoolVar(&all, "all", false, "remove the whole cache directory")
	return clean
}
