package cmd

import (
	"testing"

	"github.com/github/gh-stack/internal/config"
	"github.com/github/gh-stack/internal/git"
)

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
	ensureRerere(cfg)
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
	ensureRerere(cfg)
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
	ensureRerere(cfg)
	collectOutput(cfg, outR, errR)

	if enableCalled {
		t.Error("EnableRerere should not be called in non-interactive mode")
	}
	if declinedSaved {
		t.Error("SaveRerereDeclined should not be called in non-interactive mode")
	}
}
