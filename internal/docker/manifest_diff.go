package docker

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
)

// ExtractManifestFromImage extracts /var/vcap/bosh/manifest.yml from a Docker image
func (c *Client) ExtractManifestFromImage(ctx context.Context, imageName string) ([]byte, error) {
	c.logger.Debug(c.logTag, "Extracting manifest from image: %s", imageName)

	// Create a temporary container from the image (don't start it)
	containerConfig := &container.Config{
		Image: imageName,
	}
	resp, err := c.cli.ContainerCreate(ctx, containerConfig, nil, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary container: %w", err)
	}
	containerID := resp.ID
	defer func() {
		// Clean up the temporary container
		_ = c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	}()

	// Export the container filesystem as a tar archive
	reader, err := c.cli.ContainerExport(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to export container: %w", err)
	}
	defer reader.Close()

	// Extract the manifest file from the tar archive
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar: %w", err)
		}

		// Look for the manifest file
		if header.Name == "var/vcap/bosh/manifest.yml" || header.Name == "./var/vcap/bosh/manifest.yml" {
			c.logger.Debug(c.logTag, "Found manifest file in image: %s", header.Name)
			manifestData, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read manifest: %w", err)
			}
			return manifestData, nil
		}
	}

	return nil, fmt.Errorf("manifest file /var/vcap/bosh/manifest.yml not found in image")
}

// ShowManifestDiff compares manifests from current and new images and returns the diff
func (c *Client) ShowManifestDiff(ctx context.Context, currentImageName, newImageName string) (string, error) {
	c.logger.Info(c.logTag, "Comparing manifests between current and new images")

	// Extract manifest from current image
	currentManifest, err := c.ExtractManifestFromImage(ctx, currentImageName)
	if err != nil {
		return "", fmt.Errorf("failed to extract manifest from current image: %w", err)
	}

	// Extract manifest from new image
	newManifest, err := c.ExtractManifestFromImage(ctx, newImageName)
	if err != nil {
		return "", fmt.Errorf("failed to extract manifest from new image: %w", err)
	}

	// Parse YAML documents using ytbx
	currentDocs, err := ytbx.LoadYAMLDocuments(currentManifest)
	if err != nil {
		return "", fmt.Errorf("failed to parse current manifest: %w", err)
	}

	newDocs, err := ytbx.LoadYAMLDocuments(newManifest)
	if err != nil {
		return "", fmt.Errorf("failed to parse new manifest: %w", err)
	}

	// Create InputFile structures for dyff
	currentInput := ytbx.InputFile{
		Location:  "current",
		Documents: currentDocs,
	}

	newInput := ytbx.InputFile{
		Location:  "new",
		Documents: newDocs,
	}

	// Create dyff report
	report, err := dyff.CompareInputFiles(
		currentInput,
		newInput,
	)
	if err != nil {
		return "", fmt.Errorf("failed to compare manifests: %w", err)
	}

	// Check if there are any differences
	if len(report.Diffs) == 0 {
		c.logger.Debug(c.logTag, "No differences found in manifests")
		return "", nil
	}

	// Format and display the diff using dyff's human-readable output
	var output strings.Builder
	humanReport := dyff.HumanReport{
		Report:            report,
		OmitHeader:        false,
		NoTableStyle:      false,
		DoNotInspectCerts: true,
		UseGoPatchPaths:   false,
	}

	if err := humanReport.WriteReport(&output); err != nil {
		return "", fmt.Errorf("failed to generate diff report: %w", err)
	}

	return output.String(), nil
}
