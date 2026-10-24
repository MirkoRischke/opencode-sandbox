package main

import (
	"strings"
	"testing"
)

func TestReplacePin(t *testing.T) {
	t.Parallel()

	input := "[tools]\nmicrosandbox = \"latest\" # keep comment\n{{ if .profile }}\ngo = \"latest\"\n{{ end }}\n"
	got, err := replacePin([]byte(input), "0.7.9", "config.toml.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(input, `microsandbox = "latest"`, `microsandbox = "0.7.9"`, 1)
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReplacePinRejectsMissingOrDuplicatePins(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"[tools]\n", "microsandbox = \"latest\"\nmicrosandbox = \"0.7.8\"\n"} {
		if _, err := replacePin([]byte(input), "0.7.9", "config.toml"); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}
}
