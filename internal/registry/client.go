package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
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

// prependImageMetadata prepends image metadata to a YAML manifest.
// This allows the diff to show image ref and digest changes.
func prependImageMetadata(manifest []byte, image ImageInfo) []byte {
	// Create image metadata header
	header := fmt.Sprintf("image:\n  ref: %s\n  digest: %s\n", image.Ref, image.Digest)
	return append([]byte(header), manifest...)
}

// GetManifestDiff compares BOSH manifests from two images and returns a human-readable diff.
// Image metadata (ref, digest) is prepended to show image changes.
func (c *client) GetManifestDiff(ctx context.Context, currentImage, newImage ImageInfo) (string, error) {
	c.logger.Info(c.logTag, "Comparing manifests between %s and %s", currentImage.Ref, newImage.Ref)

	// Extract manifest from current image
	currentManifest, err := c.ExtractFileFromImage(ctx, currentImage.Ref, ManifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to extract manifest from current image: %w", err)
	}

	// Extract manifest from new image
	newManifest, err := c.ExtractFileFromImage(ctx, newImage.Ref, ManifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to extract manifest from new image: %w", err)
	}

	// Prepend image metadata to both manifests
	currentManifestWithMeta := prependImageMetadata(currentManifest, currentImage)
	newManifestWithMeta := prependImageMetadata(newManifest, newImage)

	// Parse YAML documents using ytbx
	currentDocs, err := ytbx.LoadYAMLDocuments(currentManifestWithMeta)
	if err != nil {
		return "", fmt.Errorf("failed to parse current manifest: %w", err)
	}

	newDocs, err := ytbx.LoadYAMLDocuments(newManifestWithMeta)
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

// ResolveImageRef resolves a tag-based image reference to a digest-pinned reference.
// If the input already contains a digest (@sha256:...), it returns the reference unchanged.
func (c *client) ResolveImageRef(ctx context.Context, imageRef string) (pinnedRef, digest string, err error) {
	c.logger.Debug(c.logTag, "Resolving image reference %s", imageRef)

	// Parse the image reference
	r, err := ref.New(imageRef)
	if err != nil {
		return "", "", fmt.Errorf("parsing image reference: %w", err)
	}

	// If reference already has a digest, return it as-is
	if r.Digest != "" {
		c.logger.Debug(c.logTag, "Image reference %s already has digest %s", imageRef, r.Digest)
		return imageRef, r.Digest, nil
	}

	// Create regclient with Docker credentials
	rc := regclient.New(
		regclient.WithDockerCreds(),
		regclient.WithDockerCerts(),
	)

	// Get the manifest to retrieve the digest
	manifest, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return "", "", fmt.Errorf("getting manifest: %w", err)
	}

	digest = manifest.GetDescriptor().Digest.String()

	// Build the digest-pinned reference: registry/repo@sha256:...
	pinnedRef = fmt.Sprintf("%s/%s@%s", r.Registry, r.Repository, digest)
	c.logger.Debug(c.logTag, "Resolved %s to %s", imageRef, pinnedRef)

	return pinnedRef, digest, nil
}

// FindTagsForDigest finds all tags in a repository that point to a specific digest.
// Returns tags sorted with version tags first (e.g., ["1.165", "latest"]).
func (c *client) FindTagsForDigest(ctx context.Context, imageRef string, targetDigest string) ([]string, error) {
	c.logger.Debug(c.logTag, "Finding tags for digest %s in %s", targetDigest, imageRef)

	// Parse the image reference to get registry/repository
	r, err := ref.New(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parsing image reference: %w", err)
	}

	// Create regclient with Docker credentials
	rc := regclient.New(
		regclient.WithDockerCreds(),
		regclient.WithDockerCerts(),
	)

	// List all tags for the repository
	tagList, err := rc.TagList(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	tags, err := tagList.GetTags()
	if err != nil {
		return nil, fmt.Errorf("getting tags: %w", err)
	}

	// Find tags that match the target digest
	var matchingTags []string
	for _, tag := range tags {
		// Create a reference for this tag
		tagRef, err := ref.New(fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, tag))
		if err != nil {
			c.logger.Debug(c.logTag, "Failed to parse tag reference %s: %v", tag, err)
			continue
		}

		// Get the manifest for this tag
		manifest, err := rc.ManifestGet(ctx, tagRef)
		if err != nil {
			c.logger.Debug(c.logTag, "Failed to get manifest for tag %s: %v", tag, err)
			continue
		}

		// Check if digest matches
		if manifest.GetDescriptor().Digest.String() == targetDigest {
			matchingTags = append(matchingTags, tag)
		}
	}

	// Sort tags: version tags first, then alphabetically
	sort.Slice(matchingTags, func(i, j int) bool {
		iIsVersion := isVersionTag(matchingTags[i])
		jIsVersion := isVersionTag(matchingTags[j])

		if iIsVersion && !jIsVersion {
			return true // version tags come first
		}
		if !iIsVersion && jIsVersion {
			return false
		}
		// Both same type, sort alphabetically
		return matchingTags[i] < matchingTags[j]
	})

	c.logger.Debug(c.logTag, "Found %d tags for digest %s: %v", len(matchingTags), targetDigest, matchingTags)
	return matchingTags, nil
}

// isVersionTag checks if a tag looks like a version number.
// Matches: 1.165, 1.165.0, v1.165, 1.165-alpha, 1.0.0-rc1
func isVersionTag(tag string) bool {
	versionPattern := regexp.MustCompile(`^v?\d+(\.\d+)*(-[a-zA-Z0-9.]+)?$`)
	return versionPattern.MatchString(tag)
}
