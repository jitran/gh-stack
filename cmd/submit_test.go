package cmd

import (
	"fmt"
	"io"
	"net/url"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/github/gh-stack/internal/config"
	"github.com/github/gh-stack/internal/git"
	"github.com/github/gh-stack/internal/github"
	"github.com/github/gh-stack/internal/stack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePRBody(t *testing.T) {
	tests := []struct {
		name         string
		commitBody   string
		wantContains []string
	}{
		{
			name:       "empty commit body",
			commitBody: "",
			wantContains: []string{
				"GitHub Stacks CLI",
				feedbackURL,
				"<sub>",
			},
		},
		{
			name:       "with commit body",
			commitBody: "This is a detailed description\nof the change.",
			wantContains: []string{
				"This is a detailed description\nof the change.",
				"GitHub Stacks CLI",
				"<sub>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePRBody(tt.commitBody)
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}

// newSubmitMock creates a MockOps pre-configured for submit tests.
func newSubmitMock(tmpDir string, currentBranch string) *git.MockOps {
	return &git.MockOps{
		GitDirFn:        func() (string, error) { return tmpDir, nil },
		CurrentBranchFn: func() (string, error) { return currentBranch, nil },
		ResolveRemoteFn: func(string) (string, error) { return "origin", nil },
		PushFn:          func(string, []string, bool, bool) error { return nil },
	}
}

func TestSubmit_CreatesPRsAndStack(t *testing.T) {
	s := stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1"},
			{Branch: "b2"},
		},
	}

	tmpDir := t.TempDir()
	writeStackFile(t, tmpDir, s)

	var pushCalls []pushCall
	var createdPRs []string

	mock := newSubmitMock(tmpDir, "b1")
	mock.PushFn = func(remote string, branches []string, force, atomic bool) error {
		pushCalls = append(pushCalls, pushCall{remote, branches, force, atomic})
		return nil
	}
	mock.LogRangeFn = func(base, head string) ([]git.CommitInfo, error) {
		return []git.CommitInfo{{Subject: "commit for " + head}}, nil
	}

	restore := git.SetOps(mock)
	defer restore()

	prCounter := 100
	cfg, _, errR := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{
		FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
			return nil, nil // No existing PR
		},
		CreatePRFn: func(base, head, title, body string, draft bool) (*github.PullRequest, error) {
			createdPRs = append(createdPRs, head)
			prCounter++
			return &github.PullRequest{
				Number: prCounter,
				ID:     fmt.Sprintf("PR_%d", prCounter),
				URL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prCounter),
			}, nil
		},
		CreateStackFn: func(prNumbers []int) (int, error) {
			return 42, nil
		},
	}

	cmd := SubmitCmd(cfg)
	cmd.SetArgs([]string{"--auto"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.NoError(t, err)

	// Branches should be pushed
	require.Len(t, pushCalls, 1)
	assert.Equal(t, "origin", pushCalls[0].remote)
	assert.Equal(t, []string{"b1", "b2"}, pushCalls[0].branches)

	// PRs should be created
	assert.Equal(t, []string{"b1", "b2"}, createdPRs)

	// Stack should be created
	assert.Contains(t, output, "Stack created on GitHub with 2 PRs")
	assert.Contains(t, output, "Pushed and synced 2 branches")
}

func TestSubmit_PushFailure(t *testing.T) {
	s := stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1"},
		},
	}

	tmpDir := t.TempDir()
	writeStackFile(t, tmpDir, s)

	mock := newSubmitMock(tmpDir, "b1")
	mock.PushFn = func(string, []string, bool, bool) error {
		return fmt.Errorf("remote rejected")
	}

	restore := git.SetOps(mock)
	defer restore()

	cfg, _, errR := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{}
	cmd := SubmitCmd(cfg)
	cmd.SetArgs([]string{"--auto"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.ErrorIs(t, err, ErrSilent)
	assert.Contains(t, output, "failed to push")
}

func TestSubmit_SkipsMergedBranches(t *testing.T) {
	s := stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 1, Merged: true}},
			{Branch: "b2"},
			{Branch: "b3", PullRequest: &stack.PullRequestRef{Number: 3, Merged: true}},
		},
	}

	tmpDir := t.TempDir()
	writeStackFile(t, tmpDir, s)

	var pushCalls []pushCall

	mock := newSubmitMock(tmpDir, "b2")
	mock.PushFn = func(remote string, branches []string, force, atomic bool) error {
		pushCalls = append(pushCalls, pushCall{remote, branches, force, atomic})
		return nil
	}

	restore := git.SetOps(mock)
	defer restore()

	cfg, _, errR := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{
		FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
			return &github.PullRequest{Number: 2, URL: "https://github.com/owner/repo/pull/2"}, nil
		},
	}
	cmd := SubmitCmd(cfg)
	cmd.SetArgs([]string{"--auto"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()

	cfg.Err.Close()
	_, _ = io.ReadAll(errR)

	assert.NoError(t, err)
	require.Len(t, pushCalls, 1)
	assert.Equal(t, []string{"b2"}, pushCalls[0].branches)
}

func TestSubmit_DefaultPRTitleBody(t *testing.T) {
	t.Run("single_commit", func(t *testing.T) {
		restore := git.SetOps(&git.MockOps{
			LogRangeFn: func(base, head string) ([]git.CommitInfo, error) {
				return []git.CommitInfo{
					{Subject: "Add login page", Body: "Implements the OAuth flow"},
				}, nil
			},
		})
		defer restore()

		title, body := defaultPRTitleBody("main", "feat-login")
		assert.Equal(t, "Add login page", title)
		assert.Equal(t, "Implements the OAuth flow", body)
	})

	t.Run("multiple_commits", func(t *testing.T) {
		restore := git.SetOps(&git.MockOps{
			LogRangeFn: func(base, head string) ([]git.CommitInfo, error) {
				return []git.CommitInfo{
					{Subject: "First commit"},
					{Subject: "Second commit"},
				}, nil
			},
		})
		defer restore()

		title, body := defaultPRTitleBody("main", "my-feature")
		assert.Equal(t, "my feature", title)
		assert.Equal(t, "", body)
	})
}

func TestSubmit_Humanize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-branch", "my branch"},
		{"my_branch", "my branch"},
		{"nobranch", "nobranch"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, humanize(tt.input))
		})
	}
}

func TestSyncStack_NewStack_CreateSuccess(t *testing.T) {
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	var gotNumbers []int
	mock := &github.MockClient{
		CreateStackFn: func(prNumbers []int) (int, error) {
			gotNumbers = prNumbers
			return 42, nil
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.Equal(t, []int{10, 11}, gotNumbers)
	assert.Equal(t, "42", s.ID)
	assert.Contains(t, output, "Stack created on GitHub with 2 PRs")
}

func TestSyncStack_ExistingStack_UpdateSuccess(t *testing.T) {
	s := &stack.Stack{
		ID:    "99",
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
			{Branch: "b3", PullRequest: &stack.PullRequestRef{Number: 12}},
		},
	}

	var gotStackID string
	var gotNumbers []int
	createCalled := false
	mock := &github.MockClient{
		CreateStackFn: func([]int) (int, error) {
			createCalled = true
			return 0, nil
		},
		UpdateStackFn: func(stackID string, prNumbers []int) error {
			gotStackID = stackID
			gotNumbers = prNumbers
			return nil
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.False(t, createCalled, "CreateStack should not be called when s.ID is set")
	assert.Equal(t, "99", gotStackID)
	assert.Equal(t, []int{10, 11, 12}, gotNumbers)
	assert.Contains(t, output, "Stack updated on GitHub with 3 PRs")
}

func TestSyncStack_ExistingStack_UpdateFails(t *testing.T) {
	s := &stack.Stack{
		ID:    "99",
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	mock := &github.MockClient{
		UpdateStackFn: func(string, []int) error {
			return &api.HTTPError{
				StatusCode: 422,
				Message:    "Validation failed",
				RequestURL: &url.URL{Path: "/repos/o/r/cli_internal/pulls/stacks/99"},
			}
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.Contains(t, output, "Failed to update stack")
}

func TestSyncStack_ExistingStack_Update404(t *testing.T) {
	s := &stack.Stack{
		ID:    "99",
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	var createCalled bool
	mock := &github.MockClient{
		UpdateStackFn: func(string, []int) error {
			return &api.HTTPError{
				StatusCode: 404,
				Message:    "Not Found",
				RequestURL: &url.URL{Path: "/repos/o/r/cli_internal/pulls/stacks/99"},
			}
		},
		CreateStackFn: func(prNumbers []int) (int, error) {
			createCalled = true
			return 55, nil
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.True(t, createCalled, "should fall through to CreateStack after 404")
	assert.Equal(t, "55", s.ID, "should set new stack ID from create response")
	assert.Contains(t, output, "Stack created on GitHub with 2 PRs")
}

func TestSyncStack_AlreadyStacked_OurStack(t *testing.T) {
	// All our PRs are listed as "already stacked" — this is our stack, show up-to-date.
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	mock := &github.MockClient{
		CreateStackFn: func([]int) (int, error) {
			return 0, &api.HTTPError{
				StatusCode: 422,
				Message:    "Pull requests #10, #11 are already stacked",
				RequestURL: &url.URL{Path: "/repos/o/r/cli_internal/pulls/stacks"},
			}
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.Contains(t, output, "Stack with 2 PRs is up to date")
	assert.NotContains(t, output, "different stack")
}

func TestSyncStack_AlreadyStacked_DifferentStack(t *testing.T) {
	// Only a subset of our PRs are listed — they're in a different stack.
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
			{Branch: "b3", PullRequest: &stack.PullRequestRef{Number: 12}},
		},
	}

	mock := &github.MockClient{
		CreateStackFn: func([]int) (int, error) {
			return 0, &api.HTTPError{
				StatusCode: 422,
				Message:    "Pull requests #10, #11 are already stacked",
				RequestURL: &url.URL{Path: "/repos/o/r/cli_internal/pulls/stacks"},
			}
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.Contains(t, output, "different stack")
	assert.NotContains(t, output, "up to date")
}

func TestSyncStack_InvalidChain_422(t *testing.T) {
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	mock := &github.MockClient{
		CreateStackFn: func([]int) (int, error) {
			return 0, &api.HTTPError{
				StatusCode: 422,
				Message:    "Pull requests must form a stack, where each PR's base ref is the previous PR's head ref",
				RequestURL: &url.URL{Path: "/repos/o/r/cli_internal/pulls/stacks"},
			}
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.Contains(t, output, "must form a stack")
	assert.Contains(t, output, "base branch must match")
}

func TestSyncStack_NotAvailable(t *testing.T) {
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	mock := &github.MockClient{
		CreateStackFn: func([]int) (int, error) {
			return 0, &api.HTTPError{
				StatusCode: 404,
				Message:    "Not Found",
				RequestURL: &url.URL{Path: "/repos/o/r/cli_internal/pulls/stacks"},
			}
		},
	}

	cfg, _, errR := config.NewTestConfig()
	syncStack(cfg, mock, s)

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.Contains(t, output, "not enabled")
}

func TestSyncStack_SkippedForSinglePR(t *testing.T) {
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
		},
	}

	createCalled := false
	updateCalled := false
	mock := &github.MockClient{
		CreateStackFn: func([]int) (int, error) {
			createCalled = true
			return 42, nil
		},
		UpdateStackFn: func(string, []int) error {
			updateCalled = true
			return nil
		},
	}

	cfg, _, _ := config.NewTestConfig()
	syncStack(cfg, mock, s)
	cfg.Err.Close()

	assert.False(t, createCalled, "CreateStack should not be called with fewer than 2 PRs")
	assert.False(t, updateCalled, "UpdateStack should not be called with fewer than 2 PRs")
}

func TestSyncStack_SkipsMergedBranches(t *testing.T) {
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10, Merged: true}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
			{Branch: "b3", PullRequest: &stack.PullRequestRef{Number: 12}},
		},
	}

	var gotNumbers []int
	mock := &github.MockClient{
		CreateStackFn: func(prNumbers []int) (int, error) {
			gotNumbers = prNumbers
			return 42, nil
		},
	}

	cfg, _, _ := config.NewTestConfig()
	syncStack(cfg, mock, s)
	cfg.Err.Close()

	assert.Equal(t, []int{11, 12}, gotNumbers, "should only include non-merged PRs")
}

func TestSyncStack_SkipsBranchesWithoutPR(t *testing.T) {
	s := &stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2"}, // no PR — skipped
			{Branch: "b3", PullRequest: &stack.PullRequestRef{Number: 12}},
		},
	}

	var gotNumbers []int
	mock := &github.MockClient{
		CreateStackFn: func(prNumbers []int) (int, error) {
			gotNumbers = prNumbers
			return 42, nil
		},
	}

	cfg, _, _ := config.NewTestConfig()
	syncStack(cfg, mock, s)
	cfg.Err.Close()

	assert.Equal(t, []int{10, 12}, gotNumbers, "should skip branches without PRs")
}

func TestSubmit_UpdatesBaseBranch(t *testing.T) {
	// b1's PR has base "main" but it should be "main" (correct).
	// b2's PR has base "main" but it should be "b1" (wrong — needs update).
	s := stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	tmpDir := t.TempDir()
	writeStackFile(t, tmpDir, s)

	mock := newSubmitMock(tmpDir, "b1")

	restore := git.SetOps(mock)
	defer restore()

	var updatedPRs []struct {
		number int
		base   string
	}

	cfg, _, errR := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{
		FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
			switch branch {
			case "b1":
				return &github.PullRequest{
					Number: 10, ID: "PR_10",
					URL:         "https://github.com/owner/repo/pull/10",
					BaseRefName: "main", HeadRefName: "b1",
				}, nil
			case "b2":
				return &github.PullRequest{
					Number: 11, ID: "PR_11",
					URL:         "https://github.com/owner/repo/pull/11",
					BaseRefName: "main", HeadRefName: "b2", // wrong base
				}, nil
			}
			return nil, nil
		},
		UpdatePRBaseFn: func(number int, base string) error {
			updatedPRs = append(updatedPRs, struct {
				number int
				base   string
			}{number, base})
			return nil
		},
		CreateStackFn: func(prNumbers []int) (int, error) {
			return 42, nil
		},
	}

	cmd := SubmitCmd(cfg)
	cmd.SetArgs([]string{"--auto"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.NoError(t, err)
	// b1's base is "main" which is correct — no update.
	// b2's base is "main" but should be "b1" — should be updated.
	require.Len(t, updatedPRs, 1)
	assert.Equal(t, 11, updatedPRs[0].number)
	assert.Equal(t, "b1", updatedPRs[0].base)
	assert.Contains(t, output, "Updated base branch for PR")
}

func TestSubmit_SkipsBaseUpdateWhenStacked(t *testing.T) {
	// Stack already exists (s.ID is set), so base updates should be skipped.
	s := stack.Stack{
		ID:    "99",
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 11}},
		},
	}

	tmpDir := t.TempDir()
	writeStackFile(t, tmpDir, s)

	mock := newSubmitMock(tmpDir, "b1")

	restore := git.SetOps(mock)
	defer restore()

	updateCalled := false
	cfg, _, errR := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{
		FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
			switch branch {
			case "b1":
				return &github.PullRequest{
					Number: 10, ID: "PR_10",
					URL:         "https://github.com/owner/repo/pull/10",
					BaseRefName: "main", HeadRefName: "b1",
				}, nil
			case "b2":
				return &github.PullRequest{
					Number: 11, ID: "PR_11",
					URL:         "https://github.com/owner/repo/pull/11",
					BaseRefName: "main", HeadRefName: "b2", // wrong base
				}, nil
			}
			return nil, nil
		},
		UpdatePRBaseFn: func(number int, base string) error {
			updateCalled = true
			return nil
		},
		UpdateStackFn: func(stackID string, prNumbers []int) error {
			return nil
		},
	}

	cmd := SubmitCmd(cfg)
	cmd.SetArgs([]string{"--auto"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.NoError(t, err)
	assert.False(t, updateCalled, "should not call UpdatePRBase when stack exists")
	assert.Contains(t, output, "cannot update while stacked")
}

func TestSubmit_CreatesMissingPRsAndUpdatesExisting(t *testing.T) {
	// b1 has a PR, b2 does not, b3 has a PR with wrong base.
	// Submit should create b2's PR and fix b3's base.
	s := stack.Stack{
		Trunk: stack.BranchRef{Branch: "main"},
		Branches: []stack.BranchRef{
			{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 10}},
			{Branch: "b2"},
			{Branch: "b3", PullRequest: &stack.PullRequestRef{Number: 12}},
		},
	}

	tmpDir := t.TempDir()
	writeStackFile(t, tmpDir, s)

	mock := newSubmitMock(tmpDir, "b1")
	mock.LogRangeFn = func(base, head string) ([]git.CommitInfo, error) {
		return []git.CommitInfo{{Subject: "commit for " + head}}, nil
	}

	restore := git.SetOps(mock)
	defer restore()

	var createdPRs []string
	var updatedBases []struct {
		number int
		base   string
	}

	cfg, _, errR := config.NewTestConfig()
	cfg.GitHubClientOverride = &github.MockClient{
		FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
			switch branch {
			case "b1":
				return &github.PullRequest{
					Number: 10, ID: "PR_10",
					URL:         "https://github.com/owner/repo/pull/10",
					BaseRefName: "main", HeadRefName: "b1",
				}, nil
			case "b2":
				return nil, nil // no PR
			case "b3":
				return &github.PullRequest{
					Number: 12, ID: "PR_12",
					URL:         "https://github.com/owner/repo/pull/12",
					BaseRefName: "main", HeadRefName: "b3", // wrong base — should be b2
				}, nil
			}
			return nil, nil
		},
		CreatePRFn: func(base, head, title, body string, draft bool) (*github.PullRequest, error) {
			createdPRs = append(createdPRs, head)
			return &github.PullRequest{
				Number: 11, ID: "PR_11",
				URL: "https://github.com/owner/repo/pull/11",
			}, nil
		},
		UpdatePRBaseFn: func(number int, base string) error {
			updatedBases = append(updatedBases, struct {
				number int
				base   string
			}{number, base})
			return nil
		},
		CreateStackFn: func(prNumbers []int) (int, error) {
			return 42, nil
		},
	}

	cmd := SubmitCmd(cfg)
	cmd.SetArgs([]string{"--auto"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()

	cfg.Err.Close()
	errOut, _ := io.ReadAll(errR)
	output := string(errOut)

	assert.NoError(t, err)

	// b2 should have been created
	assert.Equal(t, []string{"b2"}, createdPRs)
	assert.Contains(t, output, "Created PR")

	// b3's base should have been updated from "main" to "b2"
	require.Len(t, updatedBases, 1)
	assert.Equal(t, 12, updatedBases[0].number)
	assert.Equal(t, "b2", updatedBases[0].base)
	assert.Contains(t, output, "Updated base branch for PR")

	// Stack should be created with all 3 PRs
	assert.Contains(t, output, "Stack created on GitHub with 3 PRs")
}

// TestSubmit_BatchFetchesOpenPRs verifies that FindOpenPRsForBranches is called
// once for all active branches instead of FindPRForBranch once per branch.
// It also tests the graceful fallback: if FindOpenPRsForBranches returns nil,
// submit falls back to per-branch FindPRForBranch calls.
func TestSubmit_BatchFetchesOpenPRs(t *testing.T) {
s := stack.Stack{
Trunk: stack.BranchRef{Branch: "main"},
Branches: []stack.BranchRef{
{Branch: "b1", PullRequest: &stack.PullRequestRef{Number: 1, ID: "PR_1", URL: "u1"}},
{Branch: "b2", PullRequest: &stack.PullRequestRef{Number: 2, ID: "PR_2", URL: "u2"}},
},
}

tmpDir := t.TempDir()
writeStackFile(t, tmpDir, s)

mock := newSubmitMock(tmpDir, "b1")
restore := git.SetOps(mock)
defer restore()

var batchCalls [][]string
perBranchCalled := false

cfg, _, errR := config.NewTestConfig()
cfg.GitHubClientOverride = &github.MockClient{
// Batch fetch — the primary path.
FindOpenPRsForBranchesFn: func(branches []string) (map[string]*github.PullRequest, error) {
batchCalls = append(batchCalls, branches)
return map[string]*github.PullRequest{
"b1": {Number: 1, ID: "PR_1", URL: "u1", BaseRefName: "main", HeadRefName: "b1"},
"b2": {Number: 2, ID: "PR_2", URL: "u2", BaseRefName: "b1", HeadRefName: "b2"},
}, nil
},
// Per-branch fallback — must NOT be called when batch succeeds.
FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
perBranchCalled = true
return nil, nil
},
CreateStackFn: func(prNumbers []int) (int, error) { return 42, nil },
}

cmd := SubmitCmd(cfg)
cmd.SetArgs([]string{"--auto"})
cmd.SetOut(io.Discard)
cmd.SetErr(io.Discard)
err := cmd.Execute()

cfg.Err.Close()
_, _ = io.ReadAll(errR)

require.NoError(t, err)

// Batch query called exactly once with both active branches.
require.Len(t, batchCalls, 1)
assert.ElementsMatch(t, []string{"b1", "b2"}, batchCalls[0])

// Per-branch fallback must not have been invoked.
assert.False(t, perBranchCalled, "FindPRForBranch should not be called when batch succeeds")
}

// TestSubmit_BatchFetchFallback verifies that when FindOpenPRsForBranches
// returns an error, submit gracefully falls back to per-branch FindPRForBranch.
func TestSubmit_BatchFetchFallback(t *testing.T) {
s := stack.Stack{
Trunk: stack.BranchRef{Branch: "main"},
Branches: []stack.BranchRef{
{Branch: "b1"},
},
}

tmpDir := t.TempDir()
writeStackFile(t, tmpDir, s)

mock := newSubmitMock(tmpDir, "b1")
mock.LogRangeFn = func(base, head string) ([]git.CommitInfo, error) {
return []git.CommitInfo{{Subject: "add feature"}}, nil
}
restore := git.SetOps(mock)
defer restore()

var perBranchCalls []string
var createdPRs []string

cfg, _, errR := config.NewTestConfig()
cfg.GitHubClientOverride = &github.MockClient{
// Batch fetch fails — triggers fallback.
FindOpenPRsForBranchesFn: func(branches []string) (map[string]*github.PullRequest, error) {
return nil, fmt.Errorf("network error")
},
// Per-branch fallback should be called.
FindPRForBranchFn: func(branch string) (*github.PullRequest, error) {
perBranchCalls = append(perBranchCalls, branch)
return nil, nil // No existing PR → will be created.
},
CreatePRFn: func(base, head, title, body string, draft bool) (*github.PullRequest, error) {
createdPRs = append(createdPRs, head)
return &github.PullRequest{Number: 10, ID: "PR_10", URL: "u10"}, nil
},
CreateStackFn: func(prNumbers []int) (int, error) { return 1, nil },
}

cmd := SubmitCmd(cfg)
cmd.SetArgs([]string{"--auto"})
cmd.SetOut(io.Discard)
cmd.SetErr(io.Discard)
err := cmd.Execute()

cfg.Err.Close()
_, _ = io.ReadAll(errR)

require.NoError(t, err)

// Fallback used: per-branch lookup invoked for the active branch.
assert.Equal(t, []string{"b1"}, perBranchCalls)
assert.Equal(t, []string{"b1"}, createdPRs)
}
