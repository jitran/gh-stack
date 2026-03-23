package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/github/gh-stack/internal/config"
	"github.com/spf13/cobra"
)

func RootCmd() *cobra.Command {
	cfg := config.New()

	root := &cobra.Command{
		Use:           "stack <command>",
		Short:         "Manage stacked branches and pull requests",
		Long:          "Create, navigate, and manage stacks of branches and pull requests.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.SetOut(cfg.Out)
	root.SetErr(cfg.Err)

	// Local operations
	root.AddCommand(InitCmd(cfg))
	root.AddCommand(AddCmd(cfg))

	// Remote operations
	root.AddCommand(CheckoutCmd(cfg))
	root.AddCommand(PushCmd(cfg))
	root.AddCommand(SyncCmd(cfg))
	root.AddCommand(UnstackCmd(cfg))
	root.AddCommand(MergeCmd(cfg))

	// Helper commands
	root.AddCommand(ViewCmd(cfg))
	root.AddCommand(RebaseCmd(cfg))

	// Navigation commands
	root.AddCommand(UpCmd(cfg))
	root.AddCommand(DownCmd(cfg))
	root.AddCommand(TopCmd(cfg))
	root.AddCommand(BottomCmd(cfg))

	// Feedback
	root.AddCommand(FeedbackCmd(cfg))

	return root
}

func Execute() {
	cmd := RootCmd()
	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, ErrSilent) {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		}
		os.Exit(1)
	}
}
