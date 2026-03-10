package stack

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/prompter"
	"github.com/github/gh-stack/internal/config"
	"github.com/github/gh-stack/internal/git"
)

// ResolveStack finds the stack for the given branch, handling ambiguity when
// a branch (typically a trunk) belongs to multiple stacks. If exactly one
// stack matches, it is returned directly. If multiple stacks match, the user
// is prompted to select one and the working tree is switched to the top branch
// of the selected stack. Returns nil with no error if no stack contains the
// branch.
func (sf *StackFile) ResolveStack(branch string, cfg *config.Config) (*Stack, error) {
	stacks := sf.FindAllStacksForBranch(branch)

	switch len(stacks) {
	case 0:
		return nil, nil
	case 1:
		return stacks[0], nil
	}

	if !cfg.IsInteractive() {
		return nil, fmt.Errorf("branch %q belongs to multiple stacks; use an interactive terminal to select one", branch)
	}

	cfg.Warningf("Branch %q is the trunk of multiple stacks\n", branch)

	options := make([]string, len(stacks))
	for i, s := range stacks {
		options[i] = s.DisplayName()
	}

	p := prompter.New(os.Stdin, os.Stdout, os.Stderr)
	selected, err := p.Select("Which stack would you like to use?", "", options)
	if err != nil {
		return nil, fmt.Errorf("stack selection: %w", err)
	}

	s := stacks[selected]

	// Switch to the top branch of the selected stack so future commands
	// resolve unambiguously.
	topBranch := s.Branches[len(s.Branches)-1].Branch
	if topBranch != branch {
		if err := git.CheckoutBranch(topBranch); err != nil {
			return nil, fmt.Errorf("failed to checkout branch %s: %w", topBranch, err)
		}
		cfg.Successf("Switched to %s\n", topBranch)
	}

	return s, nil
}
