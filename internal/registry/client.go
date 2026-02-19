package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
)

const (
	// ManifestPath is the path to the BOSH manifest inside instant-bosh images
	ManifestPath = "/var/vcap/bosh/manifest.yml"
)

// client implements the Client interface using regclient for OCI registry operations.
type client struct {
	logger boshlog.Logger
	logTag string
}

// NewClient creates a new registry client.
func NewClient(logger boshlog.Logger) Client {
	return &client{
		logger: logger,
		logTag: "registryClient",
	}
}

// ExtractFileFromImage extracts a file from an OCI image by directly downloading
// it from the container registry without requiring the full image to be pulled.
func (c *client) ExtractFileFromImage(ctx context.Context, imageRef string, filePath string) ([]byte, error) {
	c.logger.Debug(c.logTag, "Extracting file %s from image %s via registry", filePath, imageRef)

	// Create regclient with Docker credentials from ~/.docker/config.json
	rc := regclient.New(regclient.WithDockerCreds())

	// Parse image reference
	r, err := ref.New(imageRef)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %s: %w", imageRef, err)
	}

	// Get the manifest
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest for %s: %w", imageRef, err)
	}

	// Get layers from the manifest
	layers, err := m.GetLayers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers from manifest: %w", err)
	}

	// Normalize the file path we're looking for
	targetPath := path.Clean(filePath)

	// Iterate through layers from top to bottom (reverse order)
	// Later layers can override files from earlier layers
	for i := len(layers) - 1; i >= 0; i-- {
		layerDesc := layers[i]
		c.logger.Debug(c.logTag, "Checking layer %s for file %s", layerDesc.Digest, filePath)

		// Process each layer in a closure to ensure proper resource cleanup
		fileData, found, err := func() ([]byte, bool, error) {
			// Get the blob (layer) from the registry
			blobStream, err := rc.BlobGet(ctx, r, layerDesc)
			if err != nil {
				c.logger.Debug(c.logTag, "Failed to get blob %s: %v", layerDesc.Digest, err)
				return nil, false, nil // Not an error, just skip this layer
			}
			defer blobStream.Close()

			// Try to decompress the blob (layers are typically gzip-compressed tar archives)
			var reader io.Reader = blobStream
			gzipReader, err := gzip.NewReader(blobStream)
			if err != nil {
				// Not gzip compressed, use the blob stream directly
				c.logger.Debug(c.logTag, "Layer %s is not gzip compressed, trying direct tar", layerDesc.Digest)
			} else {
				defer gzipReader.Close()
				reader = gzipReader
			}

			// Read the tar archive
			tarReader := tar.NewReader(reader)
			for {
				hdr, err := tarReader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					c.logger.Debug(c.logTag, "Error reading tar in layer %s: %v", layerDesc.Digest, err)
					break
				}

				// Normalize tar path (layers typically don't start with '/')
				// Handle both "var/vcap/..." and "./var/vcap/..." formats
				tarPath := "/" + strings.TrimPrefix(strings.TrimPrefix(hdr.Name, "./"), "/")

				// Check if this is the file we're looking for
				if path.Clean(tarPath) == targetPath {
					c.logger.Info(c.logTag, "Found %s in layer %s", filePath, layerDesc.Digest)

					// Read the file contents
					fileData, err := io.ReadAll(tarReader)
					if err != nil {
						return nil, false, fmt.Errorf("failed to read file %s from layer: %w", filePath, err)
					}

					return fileData, true, nil
				}
			}

			return nil, false, nil
		}()

		// Handle errors from processing the layer
		if err != nil {
			return nil, err
		}

		// If we found the file, return it
		if found {
			return fileData, nil
		}
	}

	return nil, fmt.Errorf("file %s not found in any layer of image %s", filePath, imageRef)
}

// GetManifestDiff compares BOSH manifests from two images and returns a human-readable diff.
func (c *client) GetManifestDiff(ctx context.Context, currentImageRef, newImageRef string) (string, error) {
	c.logger.Info(c.logTag, "Comparing manifests between %s and %s", currentImageRef, newImageRef)

	// Extract manifest from current image
	currentManifest, err := c.ExtractFileFromImage(ctx, currentImageRef, ManifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to extract manifest from current image: %w", err)
	}

	// Extract manifest from new image
	newManifest, err := c.ExtractFileFromImage(ctx, newImageRef, ManifestPath)
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
		OmitHeader:        true,
		NoTableStyle:      false,
		DoNotInspectCerts: true,
		UseGoPatchPaths:   false,
	}

	if err := humanReport.WriteReport(&output); err != nil {
		return "", fmt.Errorf("failed to generate diff report: %w", err)
	}

	return strings.TrimSpace(output.String()), nil
}

// GetImageDigest retrieves the digest of an image from the remote registry.
func (c *client) GetImageDigest(ctx context.Context, imageRef string) (string, error) {
	c.logger.Debug(c.logTag, "Getting digest for image %s", imageRef)

	// Create regclient with Docker credentials
	rc := regclient.New(
		regclient.WithDockerCreds(),
		regclient.WithDockerCerts(),
	)

	// Parse the image reference
	r, err := ref.New(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing image reference: %w", err)
	}

	// Get the manifest to retrieve the digest
	manifest, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return "", fmt.Errorf("getting manifest: %w", err)
	}

	digest := manifest.GetDescriptor().Digest.String()
	c.logger.Debug(c.logTag, "Image %s has digest %s", imageRef, digest)

	return digest, nil
}
