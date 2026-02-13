package cmd

import (
	"github.com/github/gh-stack/internal/config"
	"github.com/spf13/cobra"
)

type placeholderDef struct {
	Name  string
	Short string
}

var placeholderCommands = []placeholderDef{
	{"remove", "Remove a branch from a stack"},
	{"modify", "Modify a branch in a stack"},
	{"reorder", "Reorder branches in a stack"},
	{"move", "Move a branch between stacks"},
	{"fold", "Fold a branch into the branch below it"},
	{"squash", "Squash commits in a branch"},
	{"rename", "Rename a branch in a stack"},
	{"split", "Split a branch into two branches"},
}

func PlaceholderCmd(def placeholderDef, cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:    def.Name,
		Short:  def.Short,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Warningf("`gh stack %s` is not yet supported.", def.Name)
			cfg.Infof("Run `gh stack feedback` to share your thoughts on this feature.")
			return nil
		},
	}
}
