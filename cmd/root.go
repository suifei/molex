package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func Execute(ctx context.Context, version string) error {
	root := newRootCommand(version)
	root.SetContext(ctx)
	return root.Execute()
}

func newRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "molex",
		Short:         "Single-port secure TCP transit over WebSocket",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.AddCommand(
		newServeCommand(),
		newConnectCommand(),
		newWebCommand(),
		newConfigCommand(),
		&cobra.Command{
			Use:   "version",
			Short: "Print the MoleX version",
			Run: func(cmd *cobra.Command, _ []string) {
				fmt.Fprintln(cmd.OutOrStdout(), version)
			},
		},
	)
	return root
}
