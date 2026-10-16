package cmd

import "github.com/spf13/cobra"

// newRunCmd is the explicit form of the root command. It is handy when the
// command to run has the same name as one of the utd subcommands, for instance
// `utd run list` or `utd run clean`.
func newRunCmd() *cobra.Command {
	run := &cobra.Command{
		Use:                "run [flags] <command> [args...]",
		Short:              "Run a command without installing it (explicit form)",
		Long:               "Run is the explicit form of `utd <command>`. Use it when the command name collides with a utd subcommand.",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithArgs(cmd, args)
		},
	}
	return run
}
