package commands

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/stemcell"
)

// EnsureStemcellsOptions contains options for ensuring stemcells are uploaded
type EnsureStemcellsOptions struct {
	ManifestPath string   // Path to manifest file
	OpsFiles     []string // Ops files to apply
	VarFlags     []string // Variable flags (e.g., "system_domain=foo.com")
}

// EnsureStemcells ensures all required stemcells are uploaded before deployment
// 1. Run `bosh interpolate` to get final manifest
// 2. Parse stemcells from manifest
// 3. Check director for existing stemcells
// 4. Upload missing ones via CPI-specific method
func EnsureStemcells(
	ctx context.Context,
	ui UI,
	directorClient boshdir.Director,
	cpiInstance cpi.CPI,
	opts EnsureStemcellsOptions,
) error {
	// Build bosh interpolate command
	args := []string{"interpolate", opts.ManifestPath}

	for _, opsFile := range opts.OpsFiles {
		args = append(args, "-o", opsFile)
	}

	for _, varFlag := range opts.VarFlags {
		args = append(args, "-v", varFlag)
	}

	// Run bosh interpolate to get the final manifest
	cmd := exec.CommandContext(ctx, "bosh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running bosh interpolate: %w\n%s", err, stderr.String())
	}

	// Parse stemcells from manifest
	requirements, err := stemcell.ParseFromManifest(stdout.Bytes())
	if err != nil {
		return fmt.Errorf("parsing stemcells from manifest: %w", err)
	}

	if len(requirements) == 0 {
		ui.PrintLinef("No stemcells found in manifest")
		return nil
	}

	// Get existing stemcells from director
	existingStemcells, err := directorClient.Stemcells()
	if err != nil {
		return fmt.Errorf("listing existing stemcells: %w", err)
	}

	// Build a map for quick lookup
	// Key format: "os:version" for specific versions, "os:latest" needs special handling
	existingMap := make(map[string]bool)
	existingByOS := make(map[string][]boshdir.Stemcell)
	for _, s := range existingStemcells {
		key := fmt.Sprintf("%s:%s", s.OSName(), s.Version().String())
		existingMap[key] = true
		existingByOS[s.OSName()] = append(existingByOS[s.OSName()], s)
	}

	// Check each required stemcell and upload if needed
	for _, req := range requirements {
		ui.PrintLinef("Checking stemcell: %s version %s", req.OS, req.Version)

		// Check if stemcell already exists
		if req.Version != "latest" {
			key := fmt.Sprintf("%s:%s", req.OS, req.Version)
			if existingMap[key] {
				ui.PrintLinef("  Already uploaded")
				continue
			}
		} else {
			// For "latest", check if any version of this OS exists
			// The deployment will use the latest available
			if existing, ok := existingByOS[req.OS]; ok && len(existing) > 0 {
				ui.PrintLinef("  Found existing version: %s", existing[0].Version().String())
				continue
			}
		}

		// Upload the stemcell
		ui.PrintLinef("  Uploading stemcell...")
		if err := cpiInstance.UploadStemcell(ctx, directorClient, req.OS, req.Version); err != nil {
			return fmt.Errorf("uploading stemcell %s version %s: %w", req.OS, req.Version, err)
		}
		ui.PrintLinef("  Uploaded successfully")
	}

	return nil
}

// EnsureStemcellsForCF ensures all required stemcells for CF deployment are uploaded
// This is a convenience wrapper that builds the ops file and var flags for CF deployment
func EnsureStemcellsForCF(
	ctx context.Context,
	ui UI,
	directorClient boshdir.Director,
	cpiInstance cpi.CPI,
	manifestPath string,
	opsFilePaths []string,
	systemDomain string,
	routerIP string,
) error {
	varFlags := []string{
		fmt.Sprintf("system_domain=%s", systemDomain),
		fmt.Sprintf("router_static_ips=[%s]", routerIP),
	}

	return EnsureStemcells(ctx, ui, directorClient, cpiInstance, EnsureStemcellsOptions{
		ManifestPath: manifestPath,
		OpsFiles:     opsFilePaths,
		VarFlags:     varFlags,
	})
}
