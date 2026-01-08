package stemcell_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/stemcell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestParseOSFromImageRef(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		wantOS     string
		wantErr    bool
	}{
		{
			name:       "ubuntu-noble without registry",
			repository: "cloudfoundry/ubuntu-noble-stemcell",
			wantOS:     "ubuntu-noble",
			wantErr:    false,
		},
		{
			name:       "ubuntu-jammy with registry",
			repository: "ghcr.io/cloudfoundry/ubuntu-jammy-stemcell",
			wantOS:     "ubuntu-jammy",
			wantErr:    false,
		},
		{
			name:       "ubuntu-focal",
			repository: "ghcr.io/cloudfoundry/ubuntu-focal-stemcell",
			wantOS:     "ubuntu-focal",
			wantErr:    false,
		},
		{
			name:       "invalid - no stemcell suffix",
			repository: "cloudfoundry/ubuntu-noble",
			wantOS:     "",
			wantErr:    true,
		},
		{
			name:       "invalid - wrong format",
			repository: "cloudfoundry/centos-stemcell",
			wantOS:     "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOS, err := stemcell.ParseOSFromImageRef(tt.repository)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOS, gotOS)
			}
		})
	}
}

func TestBuildStemcellName(t *testing.T) {
	tests := []struct {
		os   string
		want string
	}{
		{"ubuntu-noble", "bosh-docker-ubuntu-noble"},
		{"ubuntu-jammy", "bosh-docker-ubuntu-jammy"},
		{"ubuntu-focal", "bosh-docker-ubuntu-focal"},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			got := stemcell.BuildStemcellName(tt.os)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateLightStemcell(t *testing.T) {
	info := stemcell.Info{
		Name:           "bosh-docker-ubuntu-noble",
		Version:        "1.165",
		OS:             "ubuntu-noble",
		ImageReference: "ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
		Digest:         "sha256:abc123def456",
	}

	uploadableFile, err := stemcell.CreateLightStemcell(info)
	require.NoError(t, err)
	require.NotNil(t, uploadableFile)

	// Verify we can get file info
	fileInfo, err := uploadableFile.Stat()
	require.NoError(t, err)
	assert.Equal(t, "bosh-docker-ubuntu-noble-1.165.tgz", fileInfo.Name())
	assert.Greater(t, fileInfo.Size(), int64(0))

	// Read the gzipped tarball contents
	gzipData, err := io.ReadAll(uploadableFile)
	require.NoError(t, err)

	// Decompress the gzip data
	gr, err := gzip.NewReader(bytes.NewReader(gzipData))
	require.NoError(t, err)
	defer gr.Close()

	// Verify tarball structure
	tr := tar.NewReader(gr)

	// First file should be stemcell.MF
	header, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "stemcell.MF", header.Name)

	// Read and verify manifest content
	manifestData, err := io.ReadAll(tr)
	require.NoError(t, err)

	var manifest map[string]interface{}
	err = yaml.Unmarshal(manifestData, &manifest)
	require.NoError(t, err)

	assert.Equal(t, "bosh-docker-ubuntu-noble", manifest["name"])
	assert.Equal(t, "1.165", manifest["version"])
	assert.Equal(t, 3, manifest["api_version"])
	assert.Equal(t, "ubuntu-noble", manifest["operating_system"])
	assert.Equal(t, "da39a3ee5e6b4b0d3255bfef95601890afd80709", manifest["sha1"])

	// Verify stemcell_formats
	formats, ok := manifest["stemcell_formats"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{"docker-light"}, formats)

	// Verify cloud_properties
	cloudProps, ok := manifest["cloud_properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165", cloudProps["image_reference"])
	assert.Equal(t, "sha256:abc123def456", cloudProps["digest"])

	// Second file should be empty "image" file
	header, err = tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "image", header.Name)
	assert.Equal(t, int64(0), header.Size)

	// No more files
	_, err = tr.Next()
	assert.Equal(t, io.EOF, err)
}

func TestUploadableFileClose(t *testing.T) {
	data := []byte("test data")
	uploadableFile := stemcell.NewUploadableFile(data, "test.tgz")

	err := uploadableFile.Close()
	assert.NoError(t, err)
}

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name           string
		imageRef       string
		wantRegistry   string
		wantRepository string
		wantTag        string
		wantErr        bool
	}{
		{
			name:           "full reference with tag",
			imageRef:       "ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
			wantRegistry:   "ghcr.io",
			wantRepository: "cloudfoundry/ubuntu-noble-stemcell",
			wantTag:        "1.165",
			wantErr:        false,
		},
		{
			name:           "reference without tag defaults to latest",
			imageRef:       "ghcr.io/cloudfoundry/ubuntu-noble-stemcell",
			wantRegistry:   "ghcr.io",
			wantRepository: "cloudfoundry/ubuntu-noble-stemcell",
			wantTag:        "latest",
			wantErr:        false,
		},
		{
			name:           "reference with digest",
			imageRef:       "ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165@sha256:abc123",
			wantRegistry:   "ghcr.io",
			wantRepository: "cloudfoundry/ubuntu-noble-stemcell",
			wantTag:        "1.165",
			wantErr:        false,
		},
		{
			name:           "docker.io implicit",
			imageRef:       "cloudfoundry/ubuntu-noble-stemcell:1.165",
			wantRegistry:   "docker.io",
			wantRepository: "cloudfoundry/ubuntu-noble-stemcell",
			wantTag:        "1.165",
			wantErr:        false,
		},
		{
			name:           "localhost registry",
			imageRef:       "localhost:5000/ubuntu-noble-stemcell:1.165",
			wantRegistry:   "localhost:5000",
			wantRepository: "ubuntu-noble-stemcell",
			wantTag:        "1.165",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, repository, tag, err := docker.ParseImageRef(tt.imageRef)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRegistry, registry)
				assert.Equal(t, tt.wantRepository, repository)
				assert.Equal(t, tt.wantTag, tag)
			}
		})
	}
}
