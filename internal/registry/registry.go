package registry

import (
	"context"
)

// ImageInfo contains metadata about an OCI image.
type ImageInfo struct {
	Ref    string // Full image reference (e.g., "ghcr.io/rkoster/instant-bosh:latest")
	Digest string // Image digest (e.g., "sha256:...")
}

// Client defines the interface for OCI registry operations.
// This interface is CPI-agnostic and works with any OCI-compliant registry.
//
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Client
type Client interface {
	// ExtractFileFromImage extracts a file from an OCI image without pulling the full image.
	// Uses the registry API to download only the necessary layers.
	//
	// imageRef: Full image reference (e.g., "ghcr.io/rkoster/instant-bosh:latest")
	// filePath: Absolute path inside the image filesystem (e.g., "/var/vcap/bosh/manifest.yml")
	ExtractFileFromImage(ctx context.Context, imageRef string, filePath string) ([]byte, error)

	// GetManifestDiff compares BOSH manifests between two images and returns a human-readable diff.
	// The manifest is extracted from /var/vcap/bosh/manifest.yml in each image.
	// Image metadata (ref, digest) is prepended to the manifest for visibility.
	// Returns an empty string if no differences are found.
	GetManifestDiff(ctx context.Context, currentImage, newImage ImageInfo) (string, error)

	// GetImageDigest retrieves the digest of an image from the remote registry.
	// Returns the digest in the format "sha256:...".
	GetImageDigest(ctx context.Context, imageRef string) (string, error)

	// ResolveImageRef resolves a tag-based image reference to a digest-pinned reference.
	// This allows tracking the exact image version used, even when mutable tags like "latest" are used.
	//
	// Input:  "ghcr.io/rkoster/instant-bosh:latest"
	// Output: pinnedRef="ghcr.io/rkoster/instant-bosh@sha256:abc...", digest="sha256:abc...", nil
	//
	// If the input already contains a digest (@sha256:...), it returns the reference unchanged.
	ResolveImageRef(ctx context.Context, imageRef string) (pinnedRef, digest string, err error)

	// FindTagsForDigest finds all tags in a repository that point to a specific digest.
	// This is useful for display purposes - showing which tags (like "latest", "1.165") reference the same image.
	// Returns tags sorted with version tags first (e.g., ["1.165", "latest"]).
	//
	// imageRef: Repository reference (e.g., "ghcr.io/rkoster/instant-bosh:latest" or "ghcr.io/rkoster/instant-bosh")
	// digest:   Digest to look up (e.g., "sha256:abc...")
	FindTagsForDigest(ctx context.Context, imageRef string, digest string) ([]string, error)
}
