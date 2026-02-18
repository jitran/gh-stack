package cmd

import (
	"fmt"

	"github.com/github/gh-stack/internal/config"
	"github.com/github/gh-stack/internal/git"
	"github.com/github/gh-stack/internal/stack"
	"github.com/spf13/cobra"
)

func UpCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "up [n]",
		Short: "Move up in the stack (toward the top)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := 1
			if len(args) > 0 {
				fmt.Sscanf(args[0], "%d", &n)
			}
			return runNavigate(cfg, n)
		},
	}
}

func DownCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "down [n]",
		Short: "Move down in the stack (toward the trunk)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := 1
			if len(args) > 0 {
				fmt.Sscanf(args[0], "%d", &n)
			}
			return runNavigate(cfg, -n)
		},
	}
}

func TopCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "top",
		Short: "Move to the top of the stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNavigateToEnd(cfg, true)
		},
	}
}

func BottomCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "bottom",
		Short: "Move to the bottom of the stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNavigateToEnd(cfg, false)
		},
	}
}

func runNavigate(cfg *config.Config, delta int) error {
	s, currentBranch, err := loadCurrentStack(cfg)
	if err != nil {
		return nil
	}

	idx := s.IndexOf(currentBranch)
	if idx < 0 {
		// Might be on the trunk
		if currentBranch == s.Trunk.Branch {
			if delta > 0 && len(s.Branches) > 0 {
				target := s.Branches[0].Branch
				if err := git.CheckoutBranch(target); err != nil {
					return err
				}
				cfg.Successf("Switched to %s", target)
				return nil
			}
			cfg.Printf("already at the bottom of the stack")
			return nil
		}
		cfg.Errorf("current branch %q is not in the stack", currentBranch)
		return nil
	}

	newIdx := idx + delta
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(s.Branches) {
		newIdx = len(s.Branches) - 1
	}

	if newIdx == idx {
		if delta > 0 {
			cfg.Printf("Already at the top of the stack")
		} else {
			cfg.Printf("Already at the bottom of the stack")
		}
		return nil
	}

	target := s.Branches[newIdx].Branch
	if err := git.CheckoutBranch(target); err != nil {
		return err
	}

	moved := newIdx - idx
	direction := "up"
	if moved < 0 {
		direction = "down"
		moved = -moved
	}

	cfg.Successf("Moved %s %d %s to %s", direction, moved, plural(moved, "branch", "branches"), target)
	return nil
}

func runNavigateToEnd(cfg *config.Config, top bool) error {
	s, currentBranch, err := loadCurrentStack(cfg)
	if err != nil {
		cfg.Errorf("failed to load current stack: %s", err)
		return nil
	}

	var target string
	if top {
		target = s.Branches[len(s.Branches)-1].Branch
	} else {
		target = s.Branches[0].Branch
	}

	if target == currentBranch {
		if top {
			cfg.Printf("Already at the top of the stack")
		} else {
			cfg.Printf("Already at the bottom of the stack")
		}
		return nil
	}

	if err := git.CheckoutBranch(target); err != nil {
		return err
	}

	cfg.Successf("Switched to %s", target)
	return nil
}

func loadCurrentStack(cfg *config.Config) (*stack.Stack, string, error) {
	gitDir, err := git.GitDir()
	if err != nil {
		errMsg := "not a git repository"
		cfg.Errorf("%s", errMsg)
		return nil, "", fmt.Errorf("%s", errMsg)
	}

	sf, err := stack.Load(gitDir)
	if err != nil {
		errMsg := fmt.Sprintf("failed to load stack state: %s", err)
		cfg.Errorf("%s", errMsg)
		return nil, "", fmt.Errorf("%s", errMsg)
	}

	currentBranch, err := git.CurrentBranch()
	if err != nil {
		errMsg := fmt.Sprintf("failed to get current branch: %s", err)
		cfg.Errorf("%s", errMsg)
		return nil, "", fmt.Errorf("%s", errMsg)
	}

	s := sf.FindStackForBranch(currentBranch)
	if s == nil {
		errMsg := fmt.Sprintf("current branch %q is not part of a stack", currentBranch)
		cfg.Errorf("current branch %q is not part of a stack", currentBranch)
		cfg.Printf("Checkout an existing stack using %s or create a new stack using %s", cfg.ColorCyan("gh stack checkout"), cfg.ColorCyan("gh stack init"))
		return nil, "", fmt.Errorf("%s", errMsg)
	}

	return s, currentBranch, nil
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
