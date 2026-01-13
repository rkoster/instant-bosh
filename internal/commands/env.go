package commands

import (
	"context"
	"fmt"
	"sort"
	"time"

	boshtbl "github.com/cloudfoundry/bosh-cli/v7/ui/table"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/cpi"
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

func EnvAction(ui UI, logger boshlog.Logger, cpiInstance cpi.CPI) error {
	ctx := context.Background()

	running, err := cpiInstance.IsRunning(ctx)
	if err != nil {
		return err
	}

	ui.PrintLinef("%s %s", bold("Environment:"), cpiInstance.GetContainerName())
	if running {
		ui.PrintLinef("%s Running", bold("State:"))
		ui.PrintLinef("%s %s", bold("IP:"), docker.ContainerIP)
		ui.PrintLinef("%s %s", bold("Director Port:"), docker.DirectorPort)
		ui.PrintLinef("%s %s", bold("SSH Port:"), docker.SSHPort)

		ui.PrintLinef("")
		releases, err := fetchBoshReleases(ctx, cpiInstance)
		if err != nil {
			ui.PrintLinef("Unable to fetch releases: %s", err.Error())
		} else {
			printReleasesTable(ui, releases)
		}
	} else {
		ui.PrintLinef("%s Stopped", bold("State:"))
	}

	containers, err := cpiInstance.GetContainersOnNetwork(ctx)
	if err != nil {
		ui.PrintLinef("")
		ui.PrintLinef("Unable to retrieve containers on network")
		return nil
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Created.Before(containers[j].Created)
	})

	ui.PrintLinef("")
	printContainersTable(ui, containers)

	return nil
}

func bold(s string) string {
	return fmt.Sprintf("\033[1m%s\033[0m", s)
}

func fetchBoshReleases(ctx context.Context, cpiInstance cpi.CPI) ([]Release, error) {
	manifestYAML, err := cpiInstance.ExecCommand(ctx, cpiInstance.GetContainerName(), []string{"cat", "/var/vcap/bosh/manifest.yml"})
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BoshManifest
	if err := yaml.Unmarshal([]byte(manifestYAML), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return manifest.Releases, nil
}

func printReleasesTable(ui UI, releases []Release) {
	if len(releases) == 0 {
		ui.PrintLinef("  No releases found")
		return
	}

	table := boshtbl.Table{
		Header: []boshtbl.Header{
			boshtbl.NewHeader("Release"),
			boshtbl.NewHeader("Version"),
		},
		SortBy: []boshtbl.ColumnSort{{Column: 0, Asc: true}},
	}

	for _, release := range releases {
		table.Rows = append(table.Rows, []boshtbl.Value{
			boshtbl.NewValueString(release.Name),
			boshtbl.NewValueString(release.Version),
		})
	}

	ui.PrintTable(table)
}

func printContainersTable(ui UI, containers []cpi.ContainerInfo) {
	if len(containers) == 0 {
		ui.PrintLinef("  No containers found")
		return
	}

	table := boshtbl.Table{
		Header: []boshtbl.Header{
			boshtbl.NewHeader("Container"),
			boshtbl.NewHeader("Created"),
			boshtbl.NewHeader("Network"),
		},
	}

	for _, container := range containers {
		createdStr := formatRelativeTime(container.Created)

		table.Rows = append(table.Rows, []boshtbl.Value{
			boshtbl.NewValueString(container.Name),
			boshtbl.NewValueString(createdStr),
			boshtbl.NewValueString(container.Network),
		})
	}

	ui.PrintTable(table)
}

func formatRelativeTime(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		seconds := int(duration.Seconds())
		if seconds <= 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", seconds)
	}

	if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	days := int(duration.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
