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
}
