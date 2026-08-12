package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
		RunE: func(cmd *cobra.Command, _ []string) error {
			stateDirectory, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("locate user configuration directory: %w", err)
			}
			return runWeb(cmd.Context(), webRunOptions{
				configPath:   filepath.Join(stateDirectory, "MoleX", "molex.json"),
				passwordFile: filepath.Join(stateDirectory, "MoleX", "web-password"),
				listen:       "127.0.0.1:9090",
				openBrowser:  true,
				loggerOutput: cmd.ErrOrStderr(),
			})
		},
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
