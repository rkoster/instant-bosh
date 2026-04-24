package manifests

import "path/filepath"

// BOSHDeploymentManifest returns the main bosh.yml manifest
func BOSHDeploymentManifest() ([]byte, error) {
	return boshDeploymentFS.ReadFile("bosh-deployment/bosh.yml")
}

// BOSHDeploymentOpsFile returns a specific ops file from bosh-deployment
func BOSHDeploymentOpsFile(name string) ([]byte, error) {
	path := filepath.Join("bosh-deployment", name)
	return boshDeploymentFS.ReadFile(path)
}

// BOSHLiteManifest returns the bosh-lite.yml ops file
func BOSHLiteManifest() ([]byte, error) {
	return BOSHDeploymentOpsFile("bosh-lite.yml")
}

// WardenCPIOpsFile returns the warden/cpi.yml ops file
func WardenCPIOpsFile() ([]byte, error) {
	return BOSHDeploymentOpsFile("warden/cpi.yml")
}

// WardenCloudConfig returns the warden/cloud-config.yml
func WardenCloudConfig() ([]byte, error) {
	return BOSHDeploymentOpsFile("warden/cloud-config.yml")
}

// JumpboxUserOpsFile returns the jumpbox-user.yml ops file
func JumpboxUserOpsFile() ([]byte, error) {
	return BOSHDeploymentOpsFile("jumpbox-user.yml")
}

// UAAOpsFile returns the uaa.yml ops file
func UAAOpsFile() ([]byte, error) {
	return BOSHDeploymentOpsFile("uaa.yml")
}

// CredhubOpsFile returns the credhub.yml ops file
func CredhubOpsFile() ([]byte, error) {
	return BOSHDeploymentOpsFile("credhub.yml")
}

// BOSHDevOpsFile returns the misc/bosh-dev.yml ops file
// This is the KEY file that converts create-env manifest to deploy format
func BOSHDevOpsFile() ([]byte, error) {
	return BOSHDeploymentOpsFile("misc/bosh-dev.yml")
}
