package ops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
	"gopkg.in/yaml.v3"
)

func TestDockerCPISocketMatchesContainerMount(t *testing.T) {
	makefile, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(makefile), "--ops-file ops/docker-cpi-socket.yml") {
		t.Fatal("image build must apply the Docker CPI socket ops file")
	}
	manifest := filepath.Join("..", "manifests", "bosh-deployment", "bosh.yml")
	args := []string{"int", manifest,
		"--ops-file", "../manifests/bosh-deployment/docker/cpi.yml",
		"--ops-file", "../manifests/bosh-deployment/docker/unix-sock.yml",
		"--ops-file", "docker-cpi-socket.yml"}
	interpolate := func(path string) []byte {
		t.Helper()
		output, err := exec.Command("bosh", append(append([]string{}, args...), "--path", path)...).CombinedOutput()
		if err != nil {
			t.Fatalf("interpolate %s: %v: %s", path, err, output)
		}
		return output
	}
	host := strings.TrimSpace(string(interpolate("/instance_groups/name=bosh/properties/docker_cpi/docker/host")))
	var volumes []struct {
		Path     string `yaml:"path"`
		Writable bool   `yaml:"writable"`
	}
	if err := yaml.Unmarshal(interpolate("/instance_groups/name=bosh/properties/director/cpi_additional_volumes"), &volumes); err != nil {
		t.Fatal(err)
	}
	api := &dockerfakes.FakeDockerAPI{}
	client := docker.NewTestClient(api, boshlog.NewLogger(boshlog.LevelNone), "test-image")
	if err := client.StartContainer(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, hostConfig, _, _, _ := api.ContainerCreateArgsForCall(0)
	socket := strings.TrimPrefix(host, "unix://")
	if host != "unix://"+socket || socket != "/var/run/docker.sock" {
		t.Fatalf("unexpected CPI socket %q", host)
	}
	if len(volumes) != 1 || volumes[0].Path != socket || !volumes[0].Writable {
		t.Fatalf("worker volume does not expose CPI socket: %+v", volumes)
	}
	for _, bind := range hostConfig.Binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 && parts[0] == socket && parts[1] == socket {
			return
		}
	}
	t.Fatalf("container does not mount Docker socket %s: %v", socket, hostConfig.Binds)
}
