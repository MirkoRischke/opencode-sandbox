package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestIdentifyHerdrAgent(t *testing.T) {
	if os.Getenv("OPENCODE_SANDBOX_HERDR_TEST") == "1" {
		if err := identifyHerdrAgent(); err != nil {
			t.Fatal(err)
		}
		// Exercise the same interface Herdr uses, not just os.Getenv.
		environ, err := os.ReadFile("/proc/self/environ")
		if err != nil {
			t.Fatal(err)
		}
		want := []byte("HERDR_AGENT=" + os.Getenv("OPENCODE_SANDBOX_EXPECT_AGENT"))
		for _, entry := range bytes.Split(environ, []byte{0}) {
			if bytes.Equal(entry, want) {
				return
			}
		}
		t.Fatalf("initial process environment missing %q", want)
		return
	}

	for _, tc := range []struct {
		name, herdr, agent, want string
	}{
		{name: "inside Herdr", herdr: "1", want: "opencode"},
		{name: "outside Herdr"},
		{name: "explicit hint", herdr: "1", agent: "custom", want: "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HERDR_ENV", tc.herdr)
			t.Setenv("HERDR_AGENT", tc.agent)
			t.Setenv("OPENCODE_SANDBOX_HERDR_TEST", "1")
			t.Setenv("OPENCODE_SANDBOX_EXPECT_AGENT", tc.want)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, executable, "-test.run=^TestIdentifyHerdrAgent$").CombinedOutput()
			if err != nil {
				t.Fatalf("agent identification failed: %v\n%s", err, output)
			}
		})
	}
}
