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
