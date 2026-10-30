package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDirectorDatabaseBootstrap(t *testing.T) {
	data, err := os.ReadFile("pre-start-setup.yml")
	if err != nil {
		t.Fatal(err)
	}
	var operations []struct {
		Value struct {
			Properties struct {
				Script string `yaml:"script"`
			} `yaml:"properties"`
		} `yaml:"value"`
	}
	if err := yaml.Unmarshal(data, &operations); err != nil {
		t.Fatal(err)
	}
	var setup string
	for _, op := range operations {
		if strings.Contains(op.Value.Properties.Script, "# BEGIN instant-bosh database bootstrap") {
			setup = strings.Split(strings.Split(op.Value.Properties.Script, "# BEGIN instant-bosh database bootstrap")[1], "# END instant-bosh database bootstrap")[0]
		}
	}
	if setup == "" {
		t.Fatal("bootstrap setup missing")
	}

	root := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	createDB := write("jobs/postgres/bin/create-database", "#!/bin/bash\nif oid2name -H ${HOST} -U ${USER} -d postgres -q; then :; fi\n")
	director := write("jobs/director/bin/director", "#!/bin/bash\nset -e\n${MIGRATION:-true}\nexec /var/vcap/packages/director/bin/bosh-director \\\n  -c ignored\n")
	worker := write("jobs/director/bin/worker", "#!/bin/bash\necho worker-started\n")
	scheduler := write("jobs/director/bin/scheduler", "#!/bin/bash\necho scheduler-started\n")
	ready := write("store/director/.instant-bosh-migrations-ready", "stale")
	dbReady := write("store/director/.instant-bosh-database-ready", "stale")

	// Substitute the absolute paths used by the real job only after extracting
	// the deployed script; all process execution stays inside the test directory.
	setup = strings.ReplaceAll(setup, "/var/vcap/store/director/.instant-bosh-migrations-ready", ready)
	setup = strings.ReplaceAll(setup, "/var/vcap/store/director/.instant-bosh-database-ready", dbReady)
	setup = strings.ReplaceAll(setup, "/var/vcap/jobs/director/bin/instant-bosh-wait-for-migrations", filepath.Join(root, "jobs/director/bin/instant-bosh-wait-for-migrations"))
	setup = strings.ReplaceAll(setup, "/var/vcap/jobs/director/bin/instant-bosh-wait-for-database", filepath.Join(root, "jobs/director/bin/instant-bosh-wait-for-database"))
	setup = strings.ReplaceAll(setup, "timeout 300", "timeout 4")
	runSetup := func() error {
		t.Helper()
		cmd := exec.Command("bash", "-c", "set -e\n"+setup)
		cmd.Env = append(os.Environ(), "INSTANT_BOSH_VCAP_ROOT="+root)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("setup: %s", output)
		}
		return err
	}
	if err := runSetup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Fatalf("stale readiness marker not removed: %v", err)
	}
	if _, err := os.Stat(dbReady); !os.IsNotExist(err) {
		t.Fatalf("stale database marker not removed: %v", err)
	}
	if !strings.Contains(read(createDB), "oid2name -H ${HOST} -U vcap -d postgres -q") {
		t.Fatal("bootstrap does not check the initial superuser")
	}
	if err := runSetup(); err != nil {
		t.Fatalf("setup is not idempotent: %v", err)
	}
	if strings.Count(read(worker), "source ") != 1 || strings.Count(read(director), "touch ") != 1 {
		t.Fatal("repeated setup duplicated startup gates")
	}
	if strings.Count(read(createDB), "# instant-bosh database readiness") != 1 {
		t.Fatal("repeated setup duplicated the database readiness check")
	}
	// Verify the real appended check signals only after every database accepts
	// connections as the application role.
	write("package/bin/psql", "#!/usr/bin/env bash\nexit \"${PSQL_STATUS:-0}\"\n")
	readiness := strings.Split(read(createDB), "# instant-bosh database readiness")[1]
	runReadiness := func(status string) error {
		t.Helper()
		cmd := exec.Command("bash", "-c", "DBNAMES=(config_server bosh)\nPACKAGE_DIR=\"$TEST_ROOT/package\"\nHOST=127.0.0.1\nPORT=5432\nUSER=postgres\nPASSWORD=secret\n"+readiness)
		cmd.Env = append(os.Environ(), "TEST_ROOT="+root, "PSQL_STATUS="+status)
		return cmd.Run()
	}
	if err := runReadiness("1"); err == nil {
		t.Fatal("database readiness accepted a failed connection")
	}
	if _, err := os.Stat(dbReady); !os.IsNotExist(err) {
		t.Fatal("failed database check signaled readiness")
	}
	if err := runReadiness("0"); err != nil {
		t.Fatalf("database readiness failed: %v", err)
	}

	// Simulate failure and success of the real director migration command.
	directorScript := strings.ReplaceAll(read(director), "exec /var/vcap/packages/director/bin/bosh-director \\\n  -c ignored", "exit 0")
	write("jobs/director/bin/director", directorScript)
	failed := exec.Command("bash", director)
	failed.Env = append(os.Environ(), "MIGRATION=false")
	if err := failed.Run(); err == nil {
		t.Fatal("failed migration unexpectedly succeeded")
	}
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Fatal("failed migration signaled readiness")
	}

	workerCmd := exec.Command("bash", worker)
	workerOutput := make(chan error, 1)
	go func() { workerOutput <- workerCmd.Run() }()
	select {
	case err := <-workerOutput:
		t.Fatalf("worker exited before migrations: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if output, err := exec.Command("bash", director).CombinedOutput(); err != nil {
		t.Fatalf("successful migration: %v: %s", err, output)
	}
	if err := <-workerOutput; err != nil {
		t.Fatalf("worker did not start after migrations: %v", err)
	}
	if output, err := exec.Command("bash", scheduler).CombinedOutput(); err != nil || !strings.Contains(string(output), "scheduler-started") {
		t.Fatalf("scheduler did not start after migrations: %v: %s", err, output)
	}
}
