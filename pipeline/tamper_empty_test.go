package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func emptyTamperRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	for _, name := range []string{"fhir_log.ndjson", "out_log.ndjson"} {
		if err := os.WriteFile(filepath.Join(run, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

func TestTamperRejectsRunWithoutClosedHumanSignedPrediction(t *testing.T) {
	oldOutreach, oldRBAC := OutreachOn, RBACEnforce
	t.Cleanup(func() { OutreachOn, RBACEnforce = oldOutreach, oldRBAC })
	err := tamper(t.TempDir(), "../models", emptyTamperRun(t), 4, RoleAuditor, 1)
	if err == nil {
		t.Fatal("tamper accepted a run with no closed human-signed prediction")
	}
	if !strings.Contains(err.Error(), "no closed human-signed prediction") {
		t.Fatalf("tamper error = %q, want missing-prediction diagnostic", err)
	}
}

func TestTamperCLIRejectsEmptyPredictionsWithoutPanic(t *testing.T) {
	run := emptyTamperRun(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestTamperCLIHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_TAMPER_EMPTY_HELPER=1", "TAMPER_TEST_DATA="+t.TempDir(),
		"TAMPER_TEST_MODELS=../models", "TAMPER_TEST_RUN="+run)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("tamper CLI error=%v, want exit 2; stderr=%q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no closed human-signed prediction") {
		t.Fatalf("tamper CLI stderr=%q, want missing-prediction diagnostic", stderr.String())
	}
	if strings.Contains(stderr.String(), "panic:") {
		t.Fatalf("tamper CLI panicked: %s", stderr.String())
	}
}

func TestTamperCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TAMPER_EMPTY_HELPER") != "1" {
		return
	}
	os.Args = []string{"wsi7", "tamper", "-data", os.Getenv("TAMPER_TEST_DATA"),
		"-wasl", os.Getenv("TAMPER_TEST_MODELS"), "-run", os.Getenv("TAMPER_TEST_RUN"), "-role", "auditor"}
	main()
	t.Fatal("tamper CLI returned successfully")
}
