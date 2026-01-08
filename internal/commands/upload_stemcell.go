package commands

import (
	"context"
	"fmt"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/stemcell"
)

func UploadStemcellAction(ui boshui.UI, logger boshlog.Logger, imageRef string) error {
	return UploadStemcellActionWithFactories(
		ui,
		logger,
		&docker.DefaultClientFactory{},
		&director.DefaultConfigProvider{},
		&director.DefaultDirectorFactory{},
		imageRef,
	)
}

func UploadStemcellActionWithFactories(
	ui UI,
	logger boshlog.Logger,
	clientFactory docker.ClientFactory,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
	imageRef string,
) error {
	ctx := context.Background()

	// Create Docker client
	dockerClient, err := clientFactory.NewClient(logger, "")
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	// Check if instant-bosh is running
	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if instant-bosh is running: %w", err)
	}
	if !running {
		return fmt.Errorf("instant-bosh is not running. Please start it with 'ibosh start'")
	}

	ui.PrintLinef("Resolving image: %s", imageRef)

	// Get image metadata (registry, digest, version)
	metadata, err := dockerClient.GetImageMetadata(ctx, imageRef)
	if err != nil {
		return fmt.Errorf("failed to resolve image metadata: %w", err)
	}

	ui.PrintLinef("  Registry:   %s", metadata.Registry)
	ui.PrintLinef("  Repository: %s", metadata.Repository)
	ui.PrintLinef("  Tag:        %s", metadata.Tag)
	ui.PrintLinef("  Digest:     %s", metadata.Digest)
	ui.PrintLinef("")

	// Parse OS from repository name
	os, err := stemcell.ParseOSFromImageRef(metadata.Repository)
	if err != nil {
		return fmt.Errorf("failed to parse OS from image reference: %w", err)
	}

	// Build stemcell info
	stemcellInfo := stemcell.Info{
		Name:           stemcell.BuildStemcellName(os),
		Version:        metadata.Tag,
		OS:             os,
		ImageReference: metadata.FullReference,
		Digest:         metadata.Digest,
	}

	ui.PrintLinef("Stemcell: %s version %s", stemcellInfo.Name, stemcellInfo.Version)
	ui.PrintLinef("")

	// Create light stemcell tarball
	ui.PrintLinef("Creating light stemcell...")
	uploadableFile, err := stemcell.CreateLightStemcell(stemcellInfo)
	if err != nil {
		return fmt.Errorf("failed to create light stemcell: %w", err)
	}

	// Get director configuration
	dirConfig, err := configProvider.GetDirectorConfig(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}
	defer dirConfig.Cleanup()

	// Create director client
	directorClient, err := directorFactory.NewDirector(dirConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to create director client: %w", err)
	}

	// Check if stemcell already exists
	existingStemcells, err := directorClient.Stemcells()
	if err != nil {
		return fmt.Errorf("failed to list stemcells: %w", err)
	}

	for _, s := range existingStemcells {
		if s.Name() == stemcellInfo.Name && s.Version().String() == stemcellInfo.Version {
			ui.PrintLinef("Stemcell %s/%s already uploaded", stemcellInfo.Name, stemcellInfo.Version)
			return nil
		}
	}

	// Upload stemcell
	ui.PrintLinef("Uploading to BOSH director...")
	ui.PrintLinef("")

	if err := directorClient.UploadStemcellFile(uploadableFile, false); err != nil {
		return fmt.Errorf("failed to upload stemcell: %w", err)
	}

	ui.PrintLinef("Successfully uploaded: %s/%s", stemcellInfo.Name, stemcellInfo.Version)

	return nil
}

// uploadStemcellIfNeeded uploads a stemcell if it doesn't already exist
// Returns true if the stemcell was uploaded, false if it already existed
func uploadStemcellIfNeeded(
	ctx context.Context,
	dockerClient *docker.Client,
	directorClient boshdir.Director,
	ui UI,
	logger boshlog.Logger,
	imageRef string,
	existingMap map[string]bool,
) (bool, error) {
	// Get image metadata
	metadata, err := dockerClient.GetImageMetadata(ctx, imageRef)
	if err != nil {
		return false, fmt.Errorf("resolving image metadata: %w", err)
	}

	// Parse OS from repository name
	os, err := stemcell.ParseOSFromImageRef(metadata.Repository)
	if err != nil {
		return false, fmt.Errorf("parsing OS from image reference: %w", err)
	}

	// Build stemcell info
	stemcellInfo := stemcell.Info{
		Name:           stemcell.BuildStemcellName(os),
		Version:        metadata.Tag,
		OS:             os,
		ImageReference: metadata.FullReference,
		Digest:         metadata.Digest,
	}

	// Check if already exists
	key := fmt.Sprintf("%s/%s", stemcellInfo.Name, stemcellInfo.Version)
	if existingMap[key] {
		ui.PrintLinef("%s/%s (already uploaded)", stemcellInfo.Name, stemcellInfo.Version)
		return false, nil
	}

	// Create and upload stemcell
	uploadableFile, err := stemcell.CreateLightStemcell(stemcellInfo)
	if err != nil {
		return false, fmt.Errorf("creating light stemcell: %w", err)
	}

	if err := directorClient.UploadStemcellFile(uploadableFile, false); err != nil {
		return false, fmt.Errorf("uploading stemcell: %w", err)
	}

	ui.PrintLinef("Uploaded: %s/%s", stemcellInfo.Name, stemcellInfo.Version)

	return true, nil
}
