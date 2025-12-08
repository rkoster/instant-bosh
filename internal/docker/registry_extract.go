package docker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
)

// ExtractFileFromRegistry extracts a file from a Docker image by directly downloading
// it from the container registry without requiring Docker to pull the entire image.
// This is much more efficient than the old approach of creating a temporary container.
//
// imageName: Full image reference (e.g., "ghcr.io/rkoster/instant-bosh:latest")
// fileToExtract: Absolute path inside the image filesystem (e.g., "/var/vcap/bosh/manifest.yml")
//
// Returns the file contents as a byte slice, or an error if the file is not found or
// cannot be extracted.
func (c *Client) ExtractFileFromRegistry(ctx context.Context, imageName string, fileToExtract string) ([]byte, error) {
	c.logger.Debug(c.logTag, "Extracting file %s from image %s via registry", fileToExtract, imageName)

	// Create regclient with Docker credentials from ~/.docker/config.json
	rc := regclient.New(regclient.WithDockerCreds())

	// Parse image reference
	r, err := ref.New(imageName)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %s: %w", imageName, err)
	}

	// Get the manifest
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest for %s: %w", imageName, err)
	}

	// Get layers from the manifest
	layers, err := m.GetLayers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers from manifest: %w", err)
	}

	// Normalize the file path we're looking for
	targetPath := path.Clean(fileToExtract)

	// Iterate through layers from top to bottom (reverse order)
	// Later layers can override files from earlier layers
	for i := len(layers) - 1; i >= 0; i-- {
		layerDesc := layers[i]
		c.logger.Debug(c.logTag, "Checking layer %s for file %s", layerDesc.Digest, fileToExtract)

		// Get the blob (layer) from the registry
		blobStream, err := rc.BlobGet(ctx, r, layerDesc)
		if err != nil {
			c.logger.Debug(c.logTag, "Failed to get blob %s: %v", layerDesc.Digest, err)
			continue
		}

		// Try to decompress the blob (layers are typically gzip-compressed tar archives)
		var reader io.Reader = blobStream
		gzipReader, err := gzip.NewReader(blobStream)
		if err != nil {
			// Not gzip compressed, use the blob stream directly
			c.logger.Debug(c.logTag, "Layer %s is not gzip compressed, trying direct tar", layerDesc.Digest)
		} else {
			reader = gzipReader
			defer gzipReader.Close()
		}
		defer blobStream.Close()

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
				c.logger.Info(c.logTag, "Found %s in layer %s", fileToExtract, layerDesc.Digest)

				// Read the file contents
				fileData, err := io.ReadAll(tarReader)
				if err != nil {
					return nil, fmt.Errorf("failed to read file %s from layer: %w", fileToExtract, err)
				}

				return fileData, nil
			}
		}

		// Close readers for this layer before moving to the next
		if gzipReader != nil {
			gzipReader.Close()
		}
		blobStream.Close()
	}

	return nil, fmt.Errorf("file %s not found in any layer of image %s", fileToExtract, imageName)
}

// WriteFileFromRegistry extracts a file from a Docker image registry and writes it to disk.
// This is a convenience wrapper around ExtractFileFromRegistry.
//
// imageName: Full image reference (e.g., "ghcr.io/rkoster/instant-bosh:latest")
// fileToExtract: Absolute path inside the image filesystem (e.g., "/app/mybinary")
// outputPath: Local filesystem path where the file should be written
func (c *Client) WriteFileFromRegistry(ctx context.Context, imageName string, fileToExtract string, outputPath string) error {
	c.logger.Info(c.logTag, "Extracting %s from %s to %s", fileToExtract, imageName, outputPath)

	// Extract the file from the registry
	fileData, err := c.ExtractFileFromRegistry(ctx, imageName, fileToExtract)
	if err != nil {
		return err
	}

	// Write to disk
	if err := os.WriteFile(outputPath, fileData, 0644); err != nil {
		return fmt.Errorf("failed to write file to %s: %w", outputPath, err)
	}

	c.logger.Info(c.logTag, "Successfully extracted to %s", outputPath)
	return nil
}
