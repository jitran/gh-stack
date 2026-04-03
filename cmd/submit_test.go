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
				feedbackBaseURL,
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

func TestSyncStack_AlreadyStacked_422(t *testing.T) {
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

	assert.Contains(t, output, "already part of an existing stack")
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

	assert.Contains(t, output, "not yet available")
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
			{Branch: "b2"}, // no PR yet
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
