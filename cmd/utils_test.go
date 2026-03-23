package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/github/gh-stack/internal/config"
	"github.com/github/gh-stack/internal/git"
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
