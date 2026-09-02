package models

import (
	"strings"
	"testing"
)

func TestHookValidateAuthMutualExclusion(t *testing.T) {
	base := func() *Hook {
		return &Hook{ID: "h", Name: "hook", Command: "/bin/sh -c true",
			Async: true, TimeoutSeconds: 60}
	}

	if err := base().Validate(); err != nil {
		t.Errorf("no auth should be valid, got: %v", err)
	}

	hmacOnly := base()
	hmacOnly.HMACSecret = "s"
	if err := hmacOnly.Validate(); err != nil {
		t.Errorf("hmac only should be valid, got: %v", err)
	}

	tokenOnly := base()
	tokenOnly.TriggerToken = "t"
	if err := tokenOnly.Validate(); err != nil {
		t.Errorf("token only should be valid, got: %v", err)
	}

	both := base()
	both.HMACSecret = "s"
	both.TriggerToken = "t"
	err := both.Validate()
	if err == nil {
		t.Fatal("hmac secret and trigger token together must be rejected")
	}
	want := "mutually exclusive"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should mention %q", err.Error(), want)
	}
}
