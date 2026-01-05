package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/client"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
)

// ImageMetadata contains resolved image information
type ImageMetadata struct {
	Registry      string // e.g., "ghcr.io"
	Repository    string // e.g., "cloudfoundry/ubuntu-noble-stemcell"
	Tag           string // e.g., "1.165" (resolved from "latest")
	Digest        string // e.g., "sha256:abc123..."
	FullReference string // e.g., "ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165"
}

// GetImageMetadata resolves an image reference and retrieves its metadata.
// It tries the remote registry first, then falls back to local Docker daemon.
func (c *Client) GetImageMetadata(ctx context.Context, imageRef string) (*ImageMetadata, error) {
	c.logger.Debug(c.logTag, "Resolving image metadata for %s", imageRef)

	// Try remote registry first
	metadata, err := c.getImageMetadataFromRegistry(ctx, imageRef)
	if err == nil {
		c.logger.Debug(c.logTag, "Resolved from registry: %s -> %s", imageRef, metadata.FullReference)
		return metadata, nil
	}

	c.logger.Debug(c.logTag, "Failed to resolve from registry (%v), trying local Docker daemon", err)

	// Fallback to local Docker daemon
	metadata, localErr := c.getImageMetadataFromLocal(ctx, imageRef)
	if localErr != nil {
		return nil, fmt.Errorf("failed to resolve image from registry (%v) or locally (%v)", err, localErr)
	}

	c.logger.Debug(c.logTag, "Resolved from local: %s -> %s", imageRef, metadata.FullReference)
	return metadata, nil
}

// getImageMetadataFromRegistry resolves image metadata from a remote registry
func (c *Client) getImageMetadataFromRegistry(ctx context.Context, imageRef string) (*ImageMetadata, error) {
	// Create regclient with Docker credential helper support
	rc := regclient.New(
		regclient.WithDockerCreds(),
		regclient.WithDockerCerts(),
	)

	// Parse the image reference
	r, err := ref.New(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parsing image reference: %w", err)
	}

	// Get the manifest to retrieve the digest
	manifest, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("getting manifest: %w", err)
	}

	digest := manifest.GetDescriptor().Digest.String()

	// If the tag is "latest", try to find a version tag with the same digest
	resolvedTag := r.Tag
	if r.Tag == "latest" {
		c.logger.Debug(c.logTag, "Tag is 'latest', attempting to resolve to version tag")
		versionTag, err := c.findVersionTagForDigest(ctx, rc, r, digest)
		if err != nil {
			c.logger.Debug(c.logTag, "Could not resolve version tag: %v", err)
			// Continue with "latest" if we can't find a version tag
		} else if versionTag != "" {
			resolvedTag = versionTag
			c.logger.Debug(c.logTag, "Resolved 'latest' to version tag: %s", versionTag)
		}
	}

	metadata := &ImageMetadata{
		Registry:      r.Registry,
		Repository:    r.Repository,
		Tag:           resolvedTag,
		Digest:        digest,
		FullReference: fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, resolvedTag),
	}

	return metadata, nil
}

// findVersionTagForDigest tries to find a version tag that points to the same digest
func (c *Client) findVersionTagForDigest(ctx context.Context, rc *regclient.RegClient, r ref.Ref, targetDigest string) (string, error) {
	// List all tags for the repository
	tagList, err := rc.TagList(ctx, r)
	if err != nil {
		return "", fmt.Errorf("listing tags: %w", err)
	}

	tags, err := tagList.GetTags()
	if err != nil {
		return "", fmt.Errorf("getting tags: %w", err)
	}

	// Find version tags that match the target digest
	for _, tag := range tags {
		if !isVersionTag(tag) {
			continue
		}

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
			return tag, nil
		}
	}

	return "", nil
}

// isVersionTag checks if a tag looks like a version number
// Matches: 1.165, 1.165.0, v1.165, 1.165-alpha, 1.0.0-rc1
func isVersionTag(tag string) bool {
	versionPattern := regexp.MustCompile(`^v?\d+(\.\d+)*(-[a-zA-Z0-9.]+)?$`)
	return versionPattern.MatchString(tag)
}

// getImageMetadataFromLocal resolves image metadata from local Docker daemon
func (c *Client) getImageMetadataFromLocal(ctx context.Context, imageRef string) (*ImageMetadata, error) {
	// Inspect the local image
	inspect, _, err := c.cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, fmt.Errorf("image not found locally: %s", imageRef)
		}
		return nil, fmt.Errorf("inspecting local image: %w", err)
	}

	// Extract digest from RepoDigests
	var digest string
	if len(inspect.RepoDigests) > 0 {
		// RepoDigests format: "registry/repo@sha256:..."
		parts := strings.Split(inspect.RepoDigests[0], "@")
		if len(parts) == 2 {
			digest = parts[1]
		}
	}

	if digest == "" {
		return nil, fmt.Errorf("local image has no repo digest")
	}

	// Parse the image reference to extract components
	registry, repository, tag, err := ParseImageRef(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parsing image reference: %w", err)
	}

	// If tag is "latest", try to find a version tag in RepoTags
	resolvedTag := tag
	if tag == "latest" {
		versionTag := findVersionTagInRepoTags(inspect.RepoTags)
		if versionTag != "" {
			resolvedTag = versionTag
			c.logger.Debug(c.logTag, "Resolved 'latest' to version tag from RepoTags: %s", versionTag)
		}
	}

	metadata := &ImageMetadata{
		Registry:      registry,
		Repository:    repository,
		Tag:           resolvedTag,
		Digest:        digest,
		FullReference: fmt.Sprintf("%s/%s:%s", registry, repository, resolvedTag),
	}

	return metadata, nil
}

// ParseImageRef parses an image reference into its components
func ParseImageRef(imageRef string) (registry, repository, tag string, err error) {
	// Handle digest if present (strip it for parsing)
	if strings.Contains(imageRef, "@") {
		imageRef = strings.Split(imageRef, "@")[0]
	}

	// Split by "/" to find registry and repository
	parts := strings.Split(imageRef, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid image reference format: %s", imageRef)
	}

	// Check if first part looks like a registry (has "." or ":" or is "localhost")
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		registry = parts[0]
		repository = strings.Join(parts[1:], "/")
	} else {
		// No registry, assume docker.io
		registry = "docker.io"
		repository = imageRef
	}

	// Extract tag if present (use last ":" to handle registry addresses with ports)
	idx := strings.LastIndex(repository, ":")
	if idx != -1 && idx < len(repository)-1 {
		tag = repository[idx+1:]
		repository = repository[:idx]
	} else {
		tag = "latest"
	}

	return registry, repository, tag, nil
}

// findVersionTagInRepoTags finds a version-like tag in the RepoTags list
func findVersionTagInRepoTags(repoTags []string) string {
	for _, repoTag := range repoTags {
		// RepoTags format: "registry/repo:tag" (use last ":" to handle registry addresses with ports)
		idx := strings.LastIndex(repoTag, ":")
		if idx != -1 && idx < len(repoTag)-1 {
			tag := repoTag[idx+1:]
			if isVersionTag(tag) {
				return tag
			}
		}
	}
	return ""
}
