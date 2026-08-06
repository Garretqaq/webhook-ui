package models

import "testing"

func TestScriptValidate(t *testing.T) {
	valid := Script{Name: "deploy", Interpreter: "bash", Content: "echo hi"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid script, got: %v", err)
	}

	cases := []struct {
		name   string
		script Script
	}{
		{"missing name", Script{Interpreter: "bash", Content: "echo hi"}},
		{"bad interpreter", Script{Name: "x", Interpreter: "ruby", Content: "echo hi"}},
		{"missing content", Script{Name: "x", Interpreter: "bash"}},
	}
	for _, tc := range cases {
		if err := tc.script.Validate(); err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
}

func TestIsInterpreterForOS(t *testing.T) {
	cases := []struct {
		interpreter string
		targetOS    string
		want        bool
	}{
		{"bash", TargetOSLinux, true},
		{"python3", TargetOSLinux, true},
		{"powershell", TargetOSLinux, false},
		{"powershell", TargetOSWindows, true},
		{"bash", TargetOSWindows, false},
		{"cmd", TargetOSWindows, false},
		{"bash; touch /tmp/pwn", TargetOSLinux, false},
	}
	for _, c := range cases {
		if got := IsInterpreterForOS(c.interpreter, c.targetOS); got != c.want {
			t.Errorf("IsInterpreterForOS(%q, %q) = %v, want %v", c.interpreter, c.targetOS, got, c.want)
		}
	}
}

func TestSSHHostValidateTargetOS(t *testing.T) {
	base := func() *SSHHost {
		return &SSHHost{Name: "h", Host: "1.2.3.4", Port: 22, User: "u",
			AuthType: SSHAuthPassword, Credential: "p", TargetOS: TargetOSWindows}
	}
	if err := base().Validate(); err != nil {
		t.Errorf("windows target should be valid: %v", err)
	}
	bad := base()
	bad.TargetOS = "solaris"
	if err := bad.Validate(); err == nil {
		t.Error("unknown target_os must be rejected")
	}
	empty := base()
	empty.TargetOS = ""
	if err := empty.Validate(); err == nil {
		t.Error("empty target_os must be rejected (handlers default it to linux first)")
	}
}
