package commands

import (
	"context"
	"fmt"
	"strings"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"gopkg.in/yaml.v3"
)

type Release struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	URL     string `yaml:"url"`
	SHA1    string `yaml:"sha1"`
}

type BoshManifest struct {
	Releases []Release `yaml:"releases"`
}

func EnvAction(ui boshui.UI, logger boshlog.Logger) error {
	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger, "")
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	ui.PrintLinef("instant-bosh environment:")
	if running {
		ui.PrintLinef("  State: Running")
		ui.PrintLinef("  Container: %s", docker.ContainerName)
		ui.PrintLinef("  Network: %s", docker.NetworkName)
		ui.PrintLinef("  IP: %s", docker.ContainerIP)
		ui.PrintLinef("  Director Port: %s", docker.DirectorPort)
		ui.PrintLinef("  SSH Port: %s", docker.SSHPort)
		
		// Fetch and display BOSH releases
		ui.PrintLinef("")
		ui.PrintLinef("BOSH Releases:")
		releases, err := fetchBoshReleases(ctx, dockerClient)
		if err != nil {
			ui.PrintLinef("  Unable to fetch releases: %s", err.Error())
		} else {
			printReleasesTable(ui, releases)
		}
	} else {
		ui.PrintLinef("  State: Stopped")
	}

	containers, err := dockerClient.GetContainersOnNetwork(ctx)
	if err != nil {
		ui.PrintLinef("")
		ui.PrintLinef("Containers on network: Unable to retrieve (network may not exist)")
		return nil
	}

	ui.PrintLinef("")
	ui.PrintLinef("Containers on %s network:", docker.NetworkName)
	if len(containers) == 0 {
		ui.PrintLinef("  None")
	} else {
		for _, containerName := range containers {
			ui.PrintLinef("  - %s", containerName)
		}
	}

	return nil
}

func fetchBoshReleases(ctx context.Context, dockerClient *docker.Client) ([]Release, error) {
	// Execute command to read the manifest file
	manifestYAML, err := dockerClient.ExecCommand(ctx, docker.ContainerName, []string{"cat", "/var/vcap/bosh/manifest.yml"})
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse the YAML manifest
	var manifest BoshManifest
	if err := yaml.Unmarshal([]byte(manifestYAML), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return manifest.Releases, nil
}

func printReleasesTable(ui boshui.UI, releases []Release) {
	if len(releases) == 0 {
		ui.PrintLinef("  No releases found")
		return
	}

	// Calculate column widths
	nameWidth := len("Name")
	versionWidth := len("Version")
	urlWidth := len("URL")
	sha1Width := len("SHA1")

	for _, release := range releases {
		if len(release.Name) > nameWidth {
			nameWidth = len(release.Name)
		}
		if len(release.Version) > versionWidth {
			versionWidth = len(release.Version)
		}
		if len(release.URL) > urlWidth {
			urlWidth = len(release.URL)
		}
		if len(release.SHA1) > sha1Width {
			sha1Width = len(release.SHA1)
		}
	}

	// Limit URL width to avoid overly wide tables
	maxURLWidth := 70
	if urlWidth > maxURLWidth {
		urlWidth = maxURLWidth
	}

	// Print header
	ui.PrintLinef("  %-*s  %-*s  %-*s  %-*s",
		nameWidth, "Name",
		versionWidth, "Version",
		urlWidth, "URL",
		sha1Width, "SHA1")

	// Print separator
	ui.PrintLinef("  %s  %s  %s  %s",
		strings.Repeat("-", nameWidth),
		strings.Repeat("-", versionWidth),
		strings.Repeat("-", urlWidth),
		strings.Repeat("-", sha1Width))

	// Print releases
	for _, release := range releases {
		// Truncate URL if necessary
		url := release.URL
		if len(url) > urlWidth {
			url = url[:urlWidth-3] + "..."
		}

		ui.PrintLinef("  %-*s  %-*s  %-*s  %-*s",
			nameWidth, release.Name,
			versionWidth, release.Version,
			urlWidth, url,
			sha1Width, release.SHA1)
	}
}
