package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/suifei/molex/internal/config"
)

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Create or validate a configuration"}
	command.AddCommand(newConfigInitCommand(), newConfigCheckCommand())
	return command
}

func newConfigInitCommand() *cobra.Command {
	var path, mode, role string
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
			cfg := config.Default()
			cfg.Mode = mode
			cfg.Role = role
			secret, err := config.GenerateSecret()
			if err != nil {
				return err
			}
			if mode == config.ModeRelay {
				cfg.Listen = "127.0.0.1:8080"
				cfg.Token = secret
				cfg.Secret = ""
			} else {
				cfg.Secret = secret
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", path)
			return nil
		},
	}
	command.Flags().StringVarP(&path, "config", "c", "molex.json", "configuration file")
	command.Flags().StringVar(&mode, "mode", config.ModePunch, "mode: relay or punch")
	command.Flags().StringVar(&role, "role", config.RoleEdge, "client role: edge or target")
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
