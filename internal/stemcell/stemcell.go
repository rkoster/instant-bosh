package stemcell

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"regexp"
	"strings"
)

// Info holds metadata for a light stemcell
type Info struct {
	Name           string // e.g., "bosh-docker-ubuntu-noble"
	Version        string // e.g., "1.165"
	OS             string // e.g., "ubuntu-noble"
	ImageReference string // e.g., "ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165"
	Digest         string // e.g., "sha256:abc123..."
}

// SHA1 of an empty file (used as placeholder in light stemcells)
const emptySHA1 = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

// CreateLightStemcell generates a light stemcell tarball
// Returns an UploadableFile that implements director.UploadFile
func CreateLightStemcell(info Info) (*UploadableFile, error) {
	// Generate the stemcell.MF content
	manifestContent, err := GenerateManifest(info)
	if err != nil {
		return nil, fmt.Errorf("generating manifest: %w", err)
	}

	// Create a buffer to hold the gzipped tarball
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add stemcell.MF to the tarball
	manifestHeader := &tar.Header{
		Name: "stemcell.MF",
		Size: int64(len(manifestContent)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(manifestHeader); err != nil {
		return nil, fmt.Errorf("writing manifest header: %w", err)
	}
	if _, err := tw.Write(manifestContent); err != nil {
		return nil, fmt.Errorf("writing manifest content: %w", err)
	}

	// Add empty "image" file to the tarball
	imageHeader := &tar.Header{
		Name: "image",
		Size: 0,
		Mode: 0644,
	}
	if err := tw.WriteHeader(imageHeader); err != nil {
		return nil, fmt.Errorf("writing image header: %w", err)
	}

	// Close the tar writer
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar writer: %w", err)
	}

	// Close the gzip writer
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %w", err)
	}

	// Create an UploadableFile from the buffer
	filename := fmt.Sprintf("%s-%s.tgz", info.Name, info.Version)
	return NewUploadableFile(buf.Bytes(), filename), nil
}

// ParseOSFromImageRef extracts the OS name from a repository path
// e.g., "cloudfoundry/ubuntu-noble-stemcell" -> "ubuntu-noble"
// e.g., "ghcr.io/cloudfoundry/ubuntu-jammy-stemcell" -> "ubuntu-jammy"
func ParseOSFromImageRef(repository string) (string, error) {
	// Remove registry prefix if present (e.g., ghcr.io/)
	parts := strings.Split(repository, "/")
	lastPart := parts[len(parts)-1]

	// Expected format: ubuntu-{series}-stemcell
	// Extract the OS name by removing "-stemcell" suffix
	if !strings.HasSuffix(lastPart, "-stemcell") {
		return "", fmt.Errorf("repository does not match expected format '*-stemcell': %s", repository)
	}

	osName := strings.TrimSuffix(lastPart, "-stemcell")

	// Validate it looks like ubuntu-{series}
	osPattern := regexp.MustCompile(`^ubuntu-[a-z]+$`)
	if !osPattern.MatchString(osName) {
		return "", fmt.Errorf("OS name does not match expected pattern 'ubuntu-{series}': %s", osName)
	}

	return osName, nil
}

// BuildStemcellName generates a BOSH stemcell name from OS
// e.g., "ubuntu-noble" -> "bosh-docker-ubuntu-noble"
func BuildStemcellName(os string) string {
	return fmt.Sprintf("bosh-docker-%s", os)
}

