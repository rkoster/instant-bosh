package stemcell

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ManifestData represents the structure of a stemcell.MF file
type ManifestData struct {
	Name            string            `yaml:"name"`
	Version         string            `yaml:"version"`
	APIVersion      int               `yaml:"api_version"`
	BoshProtocol    string            `yaml:"bosh_protocol"`
	SHA1            string            `yaml:"sha1"`
	OperatingSystem string            `yaml:"operating_system"`
	StemcellFormats []string          `yaml:"stemcell_formats"`
	CloudProperties CloudPropertiesData `yaml:"cloud_properties"`
}

// CloudPropertiesData represents the cloud_properties section of stemcell.MF
type CloudPropertiesData struct {
	ImageReference string `yaml:"image_reference"`
	Digest         string `yaml:"digest"`
}

// GenerateManifest creates the stemcell.MF content for a light stemcell
func GenerateManifest(info Info) ([]byte, error) {
	manifest := ManifestData{
		Name:            info.Name,
		Version:         info.Version,
		APIVersion:      3,
		BoshProtocol:    "1",
		SHA1:            emptySHA1,
		OperatingSystem: info.OS,
		StemcellFormats: []string{"docker-light"},
		CloudProperties: CloudPropertiesData{
			ImageReference: info.ImageReference,
			Digest:         info.Digest,
		},
	}

	data, err := yaml.Marshal(&manifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest to YAML: %w", err)
	}

	return data, nil
}
