package exec

import (
	"context"
	"strings"
	"testing"
)

func TestCaptureStdout(t *testing.T) {
	res, err := Capture(context.Background(), Cmd{Name: "echo", Args: []string{"hello", "world"}})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if res.Code != 0 {
		t.Fatalf("code = %d, want 0", res.Code)
	}
	if res.Stdout != "hello world" {
		t.Fatalf("stdout = %q, want %q", res.Stdout, "hello world")
	}
}

func TestCaptureNonZeroWithStderr(t *testing.T) {
	res, err := Capture(context.Background(), Cmd{
		Name: "sh",
		Args: []string{"-c", "echo oops >&2; exit 3"},
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if res.Code != 3 {
		t.Fatalf("code = %d, want 3", res.Code)
	}
	if !res.Failed() {
		t.Fatal("Failed() = false, want true")
	}
	if res.Stderr != "oops" {
		t.Fatalf("stderr = %q, want %q", res.Stderr, "oops")
	}
	if e := res.Err(); e == nil || !strings.Contains(e.Error(), "oops") {
		t.Fatalf("Err() = %v, want error containing stderr", e)
	}
}

func TestCaptureExitCodeSuccessHasNilErr(t *testing.T) {
	res, err := Capture(context.Background(), Cmd{Name: "true"})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if res.Err() != nil {
		t.Fatalf("Err() = %v, want nil for exit 0", res.Err())
	}
}

func TestCaptureEnvAndDir(t *testing.T) {
	res, err := Capture(context.Background(), Cmd{
		Name: "sh",
		Args: []string{"-c", "printf %s-%s \"$MUDEV_TEST\" \"$(basename \"$PWD\")\""},
		Env:  []string{"MUDEV_TEST=xyz"},
		Dir:  "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if res.Stdout != "xyz-tmp" {
		t.Fatalf("stdout = %q, want %q", res.Stdout, "xyz-tmp")
	}
}

func TestRunExitCode(t *testing.T) {
	code, err := Run(context.Background(), Cmd{Name: "false"})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
}

func TestStartFailureIsError(t *testing.T) {
	_, err := Capture(context.Background(), Cmd{Name: "mudev-no-such-binary-xyz"})
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestAvailable(t *testing.T) {
	if !Available("sh") {
		t.Fatal("Available(sh) = false, want true")
	}
	if Available("mudev-no-such-binary-xyz") {
		t.Fatal("Available(missing) = true, want false")
	}
}
