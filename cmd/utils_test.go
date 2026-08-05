package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/github/gh-stack/internal/config"
	"github.com/github/gh-stack/internal/git"
	"github.com/github/gh-stack/internal/github"
	"github.com/github/gh-stack/internal/stack"
	"github.com/stretchr/testify/assert"
)

func TestIsInterruptError_DirectMatch(t *testing.T) {
	if !isInterruptError(terminal.InterruptErr) {
		t.Error("expected true for terminal.InterruptErr")
	}
}

func TestIsInterruptError_Wrapped(t *testing.T) {
	// This is how the prompter library wraps the interrupt error.
	wrapped := fmt.Errorf("could not prompt: %w", terminal.InterruptErr)
	if !isInterruptError(wrapped) {
		t.Error("expected true for wrapped interrupt error")
	}
}

func TestIsInterruptError_DoubleWrapped(t *testing.T) {
	// Simulate additional wrapping by callers.
	inner := fmt.Errorf("could not prompt: %w", terminal.InterruptErr)
	outer := fmt.Errorf("stack selection: %w", inner)
	if !isInterruptError(outer) {
		t.Error("expected true for double-wrapped interrupt error")
	}
}

func TestIsInterruptError_NonInterrupt(t *testing.T) {
	if isInterruptError(errors.New("some other error")) {
		t.Error("expected false for non-interrupt error")
	}
}

func TestIsInterruptError_Nil(t *testing.T) {
	if isInterruptError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestPrintInterrupt_Output(t *testing.T) {
	cfg, outR, errR := config.NewTestConfig()
	printInterrupt(cfg)
	output := collectOutput(cfg, outR, errR)

	if !strings.Contains(output, "Received interrupt, aborting operation") {
		t.Errorf("expected interrupt message, got: %s", output)
	}
	// Should NOT contain error marker (✗)
	if strings.Contains(output, "\u2717") {
		t.Errorf("interrupt message should not use error format, got: %s", output)
	}
}

func TestErrInterrupt_IsDistinct(t *testing.T) {
	if errors.Is(errInterrupt, terminal.InterruptErr) {
		t.Error("errInterrupt sentinel should not match terminal.InterruptErr")
	}
	if !errors.Is(errInterrupt, errInterrupt) {
		t.Error("errInterrupt should match itself")
	}
}

func TestEnsureRerere_SkipsWhenAlreadyEnabled(t *testing.T) {
	enableCalled := false
	restore := git.SetOps(&git.MockOps{
		IsRerereEnabledFn: func() (bool, error) { return true, nil },
		EnableRerereFn: func() error {
			enableCalled = true
			return nil
		},
	})
	defer restore()

	cfg, outR, errR := config.NewTestConfig()
	_ = ensureRerere(cfg)
	collectOutput(cfg, outR, errR)

	if enableCalled {
		t.Error("EnableRerere should not be called when already enabled")
	}
}

func TestEnsureRerere_SkipsWhenDeclined(t *testing.T) {
	enableCalled := false
	restore := git.SetOps(&git.MockOps{
		IsRerereEnabledFn:  func() (bool, error) { return false, nil },
		IsRerereDeclinedFn: func() (bool, error) { return true, nil },
		EnableRerereFn: func() error {
			enableCalled = true
			return nil
		},
	})
	defer restore()

	cfg, outR, errR := config.NewTestConfig()
	_ = ensureRerere(cfg)
	collectOutput(cfg, outR, errR)

	if enableCalled {
		t.Error("EnableRerere should not be called when user previously declined")
	}
}

func TestEnsureRerere_SkipsWhenNonInteractive(t *testing.T) {
	enableCalled := false
	declinedSaved := false
	restore := git.SetOps(&git.MockOps{
		IsRerereEnabledFn:  func() (bool, error) { return false, nil },
		IsRerereDeclinedFn: func() (bool, error) { return false, nil },
		EnableRerereFn: func() error {
			enableCalled = true
			return nil
		},
		SaveRerereDeclinedFn: func() error {
			declinedSaved = true
			return nil
		},
	})
	defer restore()

	// NewTestConfig is non-interactive (pipes, not a TTY).
	cfg, outR, errR := config.NewTestConfig()
	_ = ensureRerere(cfg)
	collectOutput(cfg, outR, errR)

	if enableCalled {
		t.Error("EnableRerere should not be called in non-interactive mode")
	}
	if declinedSaved {
		t.Error("SaveRerereDeclined should not be called in non-interactive mode")
	}
}

func TestResolvePR_ByPRNumber(t *testing.T) {
	sf := &stack.StackFile{
		SchemaVersion: 1,
		Stacks: []stack.Stack{
			{
				Trunk: stack.BranchRef{Branch: "main"},
				Branches: []stack.BranchRef{
					{Branch: "feat-1", PullRequest: &stack.PullRequestRef{Number: 42, URL: "https://github.com/o/r/pull/42"}},
					{Branch: "feat-2", PullRequest: &stack.PullRequestRef{Number: 43, URL: "https://github.com/o/r/pull/43"}},
				},
			},
		},
	}

	cfg, _, _ := config.NewTestConfig()
	s, br, err := resolvePR(cfg, sf, "42")
	assert.NoError(t, err)
	assert.Equal(t, "feat-1", br.Branch)
	assert.Equal(t, 42, br.PullRequest.Number)
	assert.Equal(t, "main", s.Trunk.Branch)
}

func TestResolvePR_ByPRURL(t *testing.T) {
	sf := &stack.StackFile{
		SchemaVersion: 1,
		Stacks: []stack.Stack{
			{
				Trunk: stack.BranchRef{Branch: "main"},
				Branches: []stack.BranchRef{
					{Branch: "feat-1", PullRequest: &stack.PullRequestRef{Number: 42, URL: "https://github.com/o/r/pull/42"}},
				},
			},
		},
	}

	cfg, _, _ := config.NewTestConfig()
	s, br, err := resolvePR(cfg, sf, "https://github.com/o/r/pull/42")
	assert.NoError(t, err)
	assert.Equal(t, "feat-1", br.Branch)
	assert.Equal(t, "main", s.Trunk.Branch)
}

func TestResolvePR_ByBranchName(t *testing.T) {
	sf := &stack.StackFile{
		SchemaVersion: 1,
		Stacks: []stack.Stack{
			{
				Trunk: stack.BranchRef{Branch: "main"},
				Branches: []stack.BranchRef{
					{Branch: "feat-1", PullRequest: &stack.PullRequestRef{Number: 42}},
					{Branch: "feat-2", PullRequest: &stack.PullRequestRef{Number: 43}},
				},
			},
		},
	}

	cfg, _, _ := config.NewTestConfig()
	s, br, err := resolvePR(cfg, sf, "feat-2")
	assert.NoError(t, err)
	assert.Equal(t, "feat-2", br.Branch)
	assert.Equal(t, 43, br.PullRequest.Number)
	assert.Equal(t, "main", s.Trunk.Branch)
}

func TestResolvePR_NotFound(t *testing.T) {
	sf := &stack.StackFile{
		SchemaVersion: 1,
		Stacks: []stack.Stack{
			{
				Trunk:    stack.BranchRef{Branch: "main"},
				Branches: []stack.BranchRef{{Branch: "feat-1"}},
			},
		},
	}

	cfg, _, _ := config.NewTestConfig()
	_, _, err := resolvePR(cfg, sf, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no locally tracked stack found")
}

func TestResolvePR_URLPrecedesNumber(t *testing.T) {
	// A PR URL that contains number 99 should resolve via URL parsing,
	// even if PR #99 doesn't exist — the URL parser extracts the number.
	sf := &stack.StackFile{
		SchemaVersion: 1,
		Stacks: []stack.Stack{
			{
				Trunk: stack.BranchRef{Branch: "main"},
				Branches: []stack.BranchRef{
					{Branch: "feat-1", PullRequest: &stack.PullRequestRef{Number: 99, URL: "https://github.com/o/r/pull/99"}},
				},
			},
		},
	}

	cfg, _, _ := config.NewTestConfig()
	_, br, err := resolvePR(cfg, sf, "https://github.com/o/r/pull/99")
	assert.NoError(t, err)
	assert.Equal(t, 99, br.PullRequest.Number)
}

func TestParsePRURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantN  int
		wantOK bool
	}{
		{"standard URL", "https://github.com/owner/repo/pull/42", 42, true},
		{"with trailing slash", "https://github.com/owner/repo/pull/42/", 42, true},
		{"with files tab", "https://github.com/owner/repo/pull/42/files", 42, true},
		{"GHES URL", "https://ghes.example.com/owner/repo/pull/99", 99, true},
		{"GHES URL with trailing slash", "https://ghes.example.com/owner/repo/pull/7/", 7, true},
		{"not a PR URL", "https://github.com/owner/repo/issues/42", 0, false},
		{"plain number", "42", 0, false},
		{"branch name", "feat-1", 0, false},
		{"empty", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, ok := parsePRURL(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantN, n)
			}
		})
	}
}

func TestSyncStackPRs_BatchesBranches(t *testing.T) {
	// syncStackPRs should call FindPRsForBranches once with all non-merged
	// branch names, not once per branch.
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "feat-1", PullRequest: &stack.PullRequestRef{Number: 1, Merged: true}},
			{Branch: "feat-2"},
			{Branch: "feat-3"},
		},
	}

	callCount := 0
	var capturedBranches []string

	cfg, _, _ := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{
		FindPRsForBranchesFn: func(branches []string) (map[string]*github.PullRequest, error) {
			callCount++
			capturedBranches = branches
			return map[string]*github.PullRequest{
				"feat-2": {Number: 2, ID: "PR_2", URL: "https://github.com/o/r/pull/2"},
				"feat-3": {
					Number:          3,
					ID:              "PR_3",
					URL:             "https://github.com/o/r/pull/3",
					MergeQueueEntry: &github.MergeQueueEntry{ID: "MQE_3"},
				},
			}, nil
		},
	}

	syncStackPRs(cfg, s)

	assert.Equal(t, 1, callCount, "should make exactly one batch API call")
	assert.ElementsMatch(t, []string{"feat-2", "feat-3"}, capturedBranches, "should skip merged branches")

	assert.NotNil(t, s.Branches[1].PullRequest)
	assert.Equal(t, 2, s.Branches[1].PullRequest.Number)
	assert.False(t, s.Branches[1].Queued)

	assert.NotNil(t, s.Branches[2].PullRequest)
	assert.Equal(t, 3, s.Branches[2].PullRequest.Number)
	assert.True(t, s.Branches[2].Queued)
}
