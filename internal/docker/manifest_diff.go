package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
)

// ExtractManifestFromImage extracts /var/vcap/bosh/manifest.yml from a Docker image
// using the registry API. This is much more efficient than the old approach of creating
// a temporary container and exporting its filesystem.
func (c *Client) ExtractManifestFromImage(ctx context.Context, imageName string) ([]byte, error) {
	c.logger.Debug(c.logTag, "Extracting manifest from image: %s", imageName)

	// Use the new registry-based extraction method
	manifestData, err := c.ExtractFileFromRegistry(ctx, imageName, "/var/vcap/bosh/manifest.yml")
	if err != nil {
		return nil, fmt.Errorf("failed to extract manifest from registry: %w", err)
	}

	return manifestData, nil
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
