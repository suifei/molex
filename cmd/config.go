package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/suifei/molex/internal/config"
)

const placeholderToken = "mx2_replace-with-the-relay-token"

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Create or validate a configuration"}
	command.AddCommand(newConfigInitCommand(), newConfigCheckCommand())
	return command
}

func newConfigInitCommand() *cobra.Command {
	var path, mode string
	var force bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Write a starter configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !force {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("%s already exists; pass --force to replace it", path)
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			var cfg config.Config
			switch mode {
			case config.ModeRelay:
				token, err := config.GenerateToken()
				if err != nil {
					return err
				}
				id, err := config.GenerateID("tok")
				if err != nil {
					return err
				}
				cfg = config.Config{
					Mode:   config.ModeRelay,
					Listen: "127.0.0.1:8080",
					Tokens: []config.TokenEntry{{
						ID:        id,
						Token:     token,
						Note:      "default",
						CreatedAt: time.Now().UTC(),
					}},
				}
			case config.ModeTarget:
				cfg = config.Config{
					Mode:   config.ModeTarget,
					Remote: "wss://molex.example.com" + config.DefaultWebSocketPath,
					Token:  placeholderToken,
				}
			case config.ModeEdge:
				cfg = config.Config{
					Mode:   config.ModeEdge,
					Remote: "wss://molex.example.com" + config.DefaultWebSocketPath,
					Token:  placeholderToken,
				}
			default:
				return fmt.Errorf("mode must be relay, target, or edge (got %q)", mode)
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", path)
			return nil
		},
	}
	command.Flags().StringVarP(&path, "config", "c", "molex.json", "configuration file")
	command.Flags().StringVar(&mode, "mode", config.ModeEdge, "mode: relay, target, or edge")
	command.Flags().BoolVar(&force, "force", false, "replace an existing file")
	return command
}

func newConfigCheckCommand() *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:   "check",
		Short: "Validate a configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid")
			return nil
		},
	}
	command.Flags().StringVarP(&path, "config", "c", "molex.json", "configuration file")
	return command
}
