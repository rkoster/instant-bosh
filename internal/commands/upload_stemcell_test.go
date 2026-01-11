package commands_test

import (
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/errdefs"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	semver "github.com/cppforlife/go-semi-semantic/version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/director/directorfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("UploadStemcellAction", func() {
	var (
		fakeDockerAPI       *dockerfakes.FakeDockerAPI
		fakeClientFactory   *dockerfakes.FakeClientFactory
		fakeConfigProvider  *directorfakes.FakeConfigProvider
		fakeDirectorFactory *directorfakes.FakeDirectorFactory
		fakeDirector        *directorfakes.FakeDirector
		fakeUI              *commandsfakes.FakeUI
		logger              boshlog.Logger
	)

	BeforeEach(func() {
		fakeDockerAPI = &dockerfakes.FakeDockerAPI{}
		fakeClientFactory = &dockerfakes.FakeClientFactory{}
		fakeConfigProvider = &directorfakes.FakeConfigProvider{}
		fakeDirectorFactory = &directorfakes.FakeDirectorFactory{}
		fakeDirector = &directorfakes.FakeDirector{}
		fakeUI = &commandsfakes.FakeUI{}

		logger = boshlog.NewLogger(boshlog.LevelNone)

		// Configure fakeClientFactory to return a test client with fakeDockerAPI
		fakeClientFactory.NewClientStub = func(logger boshlog.Logger, customImage string) (*docker.Client, error) {
			imageName := docker.ImageName
			if customImage != "" {
				imageName = customImage
			}
			return docker.NewTestClient(fakeDockerAPI, logger, imageName), nil
		}

		// Configure fakeConfigProvider to return a default fake config
		fakeConfigProvider.GetDirectorConfigReturns(&director.Config{
			Environment:  "https://127.0.0.1:25555",
			Client:       "admin",
			ClientSecret: "fake-password",
			CACert:       "fake-cert",
		}, nil)

		// Configure fakeDirectorFactory to return fakeDirector
		fakeDirectorFactory.NewDirectorReturns(fakeDirector, nil)

		// Default: container is running
		fakeDockerAPI.ContainerListReturns([]types.Container{
			{
				Names: []string{"/instant-bosh"},
				State: "running",
				Image: "ghcr.io/rkoster/instant-bosh:latest",
			},
		}, nil)

		// Default: no existing stemcells
		fakeDirector.StemcellsReturns([]boshdir.Stemcell{}, nil)

		// Default: stemcell upload succeeds
		fakeDirector.UploadStemcellFileReturns(nil)

		// Default: image exists locally with metadata
		fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
			ID: "sha256:abc123",
			RepoTags: []string{
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
			},
			RepoDigests: []string{
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell@sha256:d4ca21a75f1ff6be382695e299257f054585143bf09762647bcb32f37be5eaf3",
			},
		}, nil, nil)

		// Default: registry metadata available
		fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
			Descriptor: ocispec.Descriptor{
				Digest: "sha256:d4ca21a75f1ff6be382695e299257f054585143bf09762647bcb32f37be5eaf3",
			},
		}, nil)

		// Default: Close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Context("when uploading a new stemcell", func() {
		It("successfully uploads the stemcell", func() {
			err := commands.UploadStemcellActionWithFactories(
				fakeUI,
				logger,
				fakeClientFactory,
				fakeConfigProvider,
				fakeDirectorFactory,
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify stemcell was uploaded
			Expect(fakeDirector.UploadStemcellFileCallCount()).To(Equal(1))

			// Verify UI messages
			foundSuccessMessage := false
			for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
				format, args := fakeUI.PrintLinefArgsForCall(i)
				message := format
				if len(args) > 0 {
					// Simple string format check
					if strings.Contains(format, "%s") || strings.Contains(format, "%v") {
						message = format // Just check the format string
					}
				}
				if strings.Contains(message, "Successfully uploaded") {
					foundSuccessMessage = true
					break
				}
			}
			Expect(foundSuccessMessage).To(BeTrue(), "Expected to find 'Successfully uploaded' message")
		})
	})

	Context("when stemcell already exists", func() {
		BeforeEach(func() {
			// Mock existing stemcell
			fakeStemcell := &directorfakes.FakeStemcell{}
			fakeStemcell.NameReturns("bosh-docker-ubuntu-noble")
			fakeStemcell.VersionReturns(semver.MustNewVersionFromString("1.165"))

			fakeDirector.StemcellsReturns([]boshdir.Stemcell{fakeStemcell}, nil)
		})

		It("skips upload and informs user", func() {
			err := commands.UploadStemcellActionWithFactories(
				fakeUI,
				logger,
				fakeClientFactory,
				fakeConfigProvider,
				fakeDirectorFactory,
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify stemcell was NOT uploaded
			Expect(fakeDirector.UploadStemcellFileCallCount()).To(Equal(0))

			// Verify UI message about already uploaded
			foundAlreadyUploadedMessage := false
			for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
				format, _ := fakeUI.PrintLinefArgsForCall(i)
				if strings.Contains(format, "already uploaded") {
					foundAlreadyUploadedMessage = true
					break
				}
			}
			Expect(foundAlreadyUploadedMessage).To(BeTrue(), "Expected to find 'already uploaded' message")
		})
	})

	Context("when instant-bosh is not running", func() {
		BeforeEach(func() {
			fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)
		})

		It("returns an error", func() {
			err := commands.UploadStemcellActionWithFactories(
				fakeUI,
				logger,
				fakeClientFactory,
				fakeConfigProvider,
				fakeDirectorFactory,
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not running"))
		})
	})

	Context("when image metadata cannot be resolved", func() {
		BeforeEach(func() {
			// Image not found locally
			fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{}, nil, errdefs.NotFound(errors.New("not found")))
			// Registry also fails
			fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{}, errors.New("registry unavailable"))
		})

		// Note: This test uses a non-existent image to trigger the error path.
		// The GetImageMetadata function tries regclient first (which we can't easily mock),
		// then falls back to Docker API. By using a clearly invalid/non-existent image,
		// we ensure both paths fail.
		It("returns an error", func() {
			err := commands.UploadStemcellActionWithFactories(
				fakeUI,
				logger,
				fakeClientFactory,
				fakeConfigProvider,
				fakeDirectorFactory,
				"ghcr.io/this-does-not-exist/ubuntu-noble-stemcell:99.999.nonexistent",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve image metadata"))
		})
	})

	Context("when director upload fails", func() {
		BeforeEach(func() {
			fakeDirector.UploadStemcellFileReturns(errors.New("upload failed"))
		})

		It("returns an error", func() {
			err := commands.UploadStemcellActionWithFactories(
				fakeUI,
				logger,
				fakeClientFactory,
				fakeConfigProvider,
				fakeDirectorFactory,
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to upload stemcell"))
		})
	})

	Context("when using latest tag", func() {
		BeforeEach(func() {
			// Mock image with latest tag resolving to specific version
			fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
				ID: "sha256:abc123",
				RepoTags: []string{
					"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:latest",
					"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165",
				},
				RepoDigests: []string{
					"ghcr.io/cloudfoundry/ubuntu-noble-stemcell@sha256:d4ca21a75f1ff6be382695e299257f054585143bf09762647bcb32f37be5eaf3",
				},
			}, nil, nil)
		})

		It("resolves to specific version and uploads", func() {
			err := commands.UploadStemcellActionWithFactories(
				fakeUI,
				logger,
				fakeClientFactory,
				fakeConfigProvider,
				fakeDirectorFactory,
				"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:latest",
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify stemcell was uploaded
			Expect(fakeDirector.UploadStemcellFileCallCount()).To(Equal(1))

			foundVersionMessage := false
			var resolvedVersion string
			for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
				format, args := fakeUI.PrintLinefArgsForCall(i)
				if strings.Contains(format, "version") && len(args) > 0 {
					for _, arg := range args {
						if str, ok := arg.(string); ok {
							resolvedVersion = str
							if str != "latest" && (len(str) > 0 && (str[0] >= '0' && str[0] <= '9' || str[0] == 'v')) {
								foundVersionMessage = true
								break
							}
						}
					}
				}
			}
			Expect(foundVersionMessage).To(BeTrue(), fmt.Sprintf("Expected 'latest' to be resolved to a version tag, but got: %s", resolvedVersion))
		})
	})
})
