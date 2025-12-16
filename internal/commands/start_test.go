package commands_test

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
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

var _ = Describe("StartAction", func() {
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

		// Configure fakeDirector to succeed with cloud-config update
		fakeDirector.UpdateCloudConfigReturns(nil)

		// Configure fakeUI to accept confirmations by default
		fakeUI.AskForConfirmationReturns(nil)

		// Default: no containers running
		fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)

		// Default: image doesn't exist locally
		fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{}, nil, errdefs.NotFound(errors.New("not found")))

		// Default: image pull succeeds
		fakeDockerAPI.ImagePullReturns(io.NopCloser(strings.NewReader("{}")), nil)

		// Default: network doesn't exist
		fakeDockerAPI.NetworkInspectReturns(network.Inspect{}, nil)

		// Default: network creation succeeds
		fakeDockerAPI.NetworkCreateReturns(network.CreateResponse{}, nil)

		// Default: volume creation succeeds
		fakeDockerAPI.VolumeCreateReturns(volume.Volume{}, nil)

		// Default: volumes don't exist initially
		fakeDockerAPI.VolumeInspectReturns(volume.Volume{}, errdefs.NotFound(errors.New("not found")))

		// Default: container creation succeeds
		fakeDockerAPI.ContainerCreateReturns(container.CreateResponse{ID: "test-container-id"}, nil)

		// Default: container start succeeds
		fakeDockerAPI.ContainerStartReturns(nil)

		// Default: container inspect returns healthy container
		fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Running: true,
					Health:  &types.Health{Status: "healthy"},
				},
			},
		}, nil)

		// Default: distribution inspect returns manifest descriptor
		fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
			Descriptor: ocispec.Descriptor{
				Digest: "sha256:abc123",
			},
		}, nil)

		// Default: DaemonHost returns unix socket
		fakeDockerAPI.DaemonHostReturns("unix:///var/run/docker.sock")

		// Default: Close succeeds
		fakeDockerAPI.CloseReturns(nil)

		// Default: ContainerLogs returns empty reader
		fakeDockerAPI.ContainerLogsReturns(io.NopCloser(strings.NewReader("")), nil)
	})

	Describe("upgrade scenario", func() {
		Context("when container is running with a different image and user accepts upgrade", func() {
			BeforeEach(func() {
				// Container is running
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					// After new container started: return new running container
					// Note: Check ContainerStartCallCount() >= 2 because we stop then restart
					if fakeDockerAPI.ContainerStartCallCount() >= 2 || (fakeDockerAPI.ContainerStartCallCount() >= 1 && fakeDockerAPI.ContainerStopCallCount() > 0) {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					// After stop but before restart: container is gone
					if fakeDockerAPI.ContainerStopCallCount() > 0 {
						return []types.Container{}, nil
					}
					// Initially: old container running
					return []types.Container{
						{
							Names: []string{"/instant-bosh"},
							State: "running",
							Image: "ghcr.io/rkoster/instant-bosh:old",
						},
					}, nil
				}

				// Container inspect shows it's running with different image
				fakeDockerAPI.ContainerInspectStub = func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
					// After container is created, return healthy state
					if fakeDockerAPI.ContainerCreateCallCount() > 0 {
						return types.ContainerJSON{
							ContainerJSONBase: &types.ContainerJSONBase{
								Image: "sha256:new-image-id-full",
								State: &types.ContainerState{
									Running: true,
									Health:  &types.Health{Status: "healthy"},
								},
							},
						}, nil
					}
					// Before creation, show old container
					return types.ContainerJSON{
						ContainerJSONBase: &types.ContainerJSONBase{
							Image: "sha256:old-image-id-full",
							State: &types.ContainerState{Running: true},
						},
						Config: &container.Config{
							Image: "ghcr.io/rkoster/instant-bosh:old",
						},
					}, nil
				}

				// Target image exists locally
				fakeDockerAPI.ImageInspectWithRawStub = func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
					if strings.Contains(imageID, "old") {
						return types.ImageInspect{ID: "old-image-id"}, nil, nil
					}
					return types.ImageInspect{ID: "new-image-id"}, nil, nil
				}

				// User accepts the upgrade
				fakeUI.AskForConfirmationReturns(nil)

				// Container stop succeeds
				fakeDockerAPI.ContainerStopReturns(nil)
			})

			It("displays checking for updates message and upgrades the container", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify UI messages
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))

				// Check that upgrade message was displayed
				foundUpgradeMessage := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if strings.Contains(format, "Upgrading to new image") {
						foundUpgradeMessage = true
						break
					}
				}
				Expect(foundUpgradeMessage).To(BeTrue(), "Expected to find 'Upgrading to new image' message")

				// Verify container was stopped
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(1))

				// Verify new container was created
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))

				// Verify cloud-config was applied
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))
			})
		})

		Context("when container is running with different image but user declines upgrade", func() {
			BeforeEach(func() {
				// Container is running
				fakeDockerAPI.ContainerListReturns([]types.Container{
					{
						Names: []string{"/instant-bosh"},
						State: "running",
						Image: "ghcr.io/rkoster/instant-bosh:old",
					},
				}, nil)

				// Container inspect shows it's running with different image
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						Image: "sha256:old-image-id-full",
						State: &types.ContainerState{Running: true},
					},
					Config: &container.Config{
						Image: "ghcr.io/rkoster/instant-bosh:old",
					},
				}, nil)

				// Target image exists locally
				fakeDockerAPI.ImageInspectWithRawStub = func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
					if strings.Contains(imageID, "old") {
						return types.ImageInspect{ID: "old-image-id"}, nil, nil
					}
					return types.ImageInspect{ID: "new-image-id"}, nil, nil
				}

				// User declines the upgrade
				fakeUI.AskForConfirmationReturns(errors.New("user declined"))
			})

			It("cancels upgrade without making changes", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify container was NOT stopped
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(0))

				// Verify no new container was created
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(0))

				// Verify cloud-config was NOT applied
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(0))
			})
		})
	})

	Describe("fresh start scenario", func() {
		Context("when no container exists and image needs to be pulled", func() {
			BeforeEach(func() {
				// Container list starts empty, then returns running container after start
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					if fakeDockerAPI.ContainerStartCallCount() > 0 {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					return []types.Container{}, nil
				}

				// Image doesn't exist locally, needs pull
				callCount := 0
				fakeDockerAPI.ImageInspectWithRawStub = func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
					callCount++
					if callCount == 1 {
						// First call: image not found
						return types.ImageInspect{}, nil, errdefs.NotFound(errors.New("not found"))
					}
					// After pull: image exists with an ID
					return types.ImageInspect{
						ID:          "sha256:new-image-id",
						RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:abc123"},
					}, nil, nil
				}

				// Container inspection returns healthy after creation
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							Running: true,
							Health:  &types.Health{Status: "healthy"},
						},
					},
				}, nil)
			})

			It("pulls image and starts container", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify image was pulled
				Expect(fakeDockerAPI.ImagePullCallCount()).To(Equal(1))

				// Verify container was created and started
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))
				Expect(fakeDockerAPI.ContainerStartCallCount()).To(Equal(1))

				// Verify cloud-config was applied
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))

				// Verify UI messages were displayed
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("when image exists and update is available", func() {
			BeforeEach(func() {
				// Container list starts empty, then returns running container after start
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					if fakeDockerAPI.ContainerStartCallCount() > 0 {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					return []types.Container{}, nil
				}

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:local123"},
				}, nil, nil)

				// Remote has different digest (update available)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:remote456",
					},
				}, nil)

				// Container inspection returns healthy
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							Running: true,
							Health:  &types.Health{Status: "healthy"},
						},
					},
				}, nil)
			})

			It("checks for updates, pulls newer image, and starts container", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify update check was performed
				Expect(fakeDockerAPI.DistributionInspectCallCount()).To(Equal(1))

				// Verify newer image was pulled
				Expect(fakeDockerAPI.ImagePullCallCount()).To(Equal(1))

				// Verify container was created and started
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))
				Expect(fakeDockerAPI.ContainerStartCallCount()).To(Equal(1))

				// Verify cloud-config was applied
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))

				// Check for update message in UI
				foundUpdateMessage := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					// Simple check for update-related keywords
					if strings.Contains(format, "newer revision") {
						foundUpdateMessage = true
						break
					}
				}
				Expect(foundUpdateMessage).To(BeTrue(), "Expected to find update availability message")
			})
		})

		Context("when image is at latest version", func() {
			BeforeEach(func() {
				// Container list starts empty, then returns running container after start
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					if fakeDockerAPI.ContainerStartCallCount() > 0 {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					return []types.Container{}, nil
				}

				// Image exists locally with specific digest
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:same123"},
				}, nil, nil)

				// Remote has same digest (at latest version)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:same123",
					},
				}, nil)

				// Container inspection returns healthy
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							Running: true,
							Health:  &types.Health{Status: "healthy"},
						},
					},
				}, nil)
			})

			It("displays 'at latest version' message and starts container", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify update check was performed
				Expect(fakeDockerAPI.DistributionInspectCallCount()).To(Equal(1))

				// Verify image was NOT pulled (already at latest)
				Expect(fakeDockerAPI.ImagePullCallCount()).To(Equal(0))

				// Verify container was created and started
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))
				Expect(fakeDockerAPI.ContainerStartCallCount()).To(Equal(1))

				// Check for "latest version" message in UI
				foundLatestMessage := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if strings.Contains(format, "latest version") {
						foundLatestMessage = true
						break
					}
				}
				Expect(foundLatestMessage).To(BeTrue(), "Expected to find 'at latest version' message")
			})
		})
	})

	Describe("container already running scenarios", func() {
		Context("when container is already running with same image", func() {
			BeforeEach(func() {
				// Container is running
				fakeDockerAPI.ContainerListReturns([]types.Container{
					{
						Names: []string{"/instant-bosh"},
						State: "running",
						Image: docker.ImageName,
					},
				}, nil)

				// Container inspect shows it's running with same image
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						Image: "sha256:current-image-id",
						State: &types.ContainerState{Running: true},
					},
					Config: &container.Config{
						Image: docker.ImageName,
					},
				}, nil)

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID: "sha256:current-image-id",
				}, nil, nil)
			})

			It("displays already running message without recreating container", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify container was NOT stopped or recreated
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(0))
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(0))

				// Verify cloud-config was NOT applied
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(0))

				// Check for "already running" message
				foundAlreadyRunning := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if strings.Contains(format, "already running") {
						foundAlreadyRunning = true
						break
					}
				}
				Expect(foundAlreadyRunning).To(BeTrue(), "Expected to find 'already running' message")
			})
		})
	})

	Context("factory pattern implementation", func() {
			It("successfully implements factory pattern for all dependencies", func() {
			// Verify all factories are properly instantiated
			Expect(fakeClientFactory).NotTo(BeNil())
			Expect(fakeConfigProvider).NotTo(BeNil())
			Expect(fakeDirectorFactory).NotTo(BeNil())
			Expect(fakeUI).NotTo(BeNil())

			// The pattern is working - factories successfully create test doubles
			client, err := fakeClientFactory.NewClient(logger, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())

			config, err := fakeConfigProvider.GetDirectorConfig(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(config).NotTo(BeNil())

			director, err := fakeDirectorFactory.NewDirector(config, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(director).NotTo(BeNil())
		})
	})

	Describe("resource existence checks", func() {
		Context("when volumes and network already exist", func() {
			BeforeEach(func() {
				// Container list starts empty, then returns running container after start
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					if fakeDockerAPI.ContainerStartCallCount() > 0 {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					return []types.Container{}, nil
				}

				// Volumes exist
				fakeDockerAPI.VolumeInspectReturns(volume.Volume{
					Name: "test-volume",
				}, nil)

				// Network exists
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{
					Name: docker.NetworkName,
				}, nil)

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:same123"},
				}, nil, nil)

				// Remote has same digest (at latest version)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:same123",
					},
				}, nil)

				// Container inspection returns healthy
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							Running: true,
							Health:  &types.Health{Status: "healthy"},
						},
					},
				}, nil)
			})

			It("displays 'Using existing' messages for volumes and network", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify volumes were NOT created
				Expect(fakeDockerAPI.VolumeCreateCallCount()).To(Equal(0))

				// Verify network was NOT created
				Expect(fakeDockerAPI.NetworkCreateCallCount()).To(Equal(0))

				// Check for "Using existing" messages in UI
				foundUsingVolumesMessage := false
				foundUsingNetworkMessage := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if strings.Contains(format, "Using existing volumes") {
						foundUsingVolumesMessage = true
					}
					if strings.Contains(format, "Using existing network") {
						foundUsingNetworkMessage = true
					}
				}
				Expect(foundUsingVolumesMessage).To(BeTrue(), "Expected to find 'Using existing volumes' message")
				Expect(foundUsingNetworkMessage).To(BeTrue(), "Expected to find 'Using existing network' message")

				// Verify container was created and started
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))
				Expect(fakeDockerAPI.ContainerStartCallCount()).To(Equal(1))
			})
		})

		Context("when volumes and network don't exist", func() {
			BeforeEach(func() {
				// Container list starts empty, then returns running container after start
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					if fakeDockerAPI.ContainerStartCallCount() > 0 {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					return []types.Container{}, nil
				}

				// Volumes don't exist
				fakeDockerAPI.VolumeInspectReturns(volume.Volume{}, errdefs.NotFound(errors.New("not found")))

				// Network doesn't exist
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{}, errdefs.NotFound(errors.New("not found")))

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:same123"},
				}, nil, nil)

				// Remote has same digest (at latest version)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:same123",
					},
				}, nil)

				// Container inspection returns healthy
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							Running: true,
							Health:  &types.Health{Status: "healthy"},
						},
					},
				}, nil)
			})

			It("displays 'Creating' messages and creates volumes and network", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify volumes were created
				Expect(fakeDockerAPI.VolumeCreateCallCount()).To(Equal(2))

				// Verify network was created
				Expect(fakeDockerAPI.NetworkCreateCallCount()).To(Equal(1))

				// Check for "Creating" messages in UI
				foundCreatingVolumesMessage := false
				foundCreatingNetworkMessage := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if strings.Contains(format, "Creating volumes") {
						foundCreatingVolumesMessage = true
					}
					if strings.Contains(format, "Creating network") {
						foundCreatingNetworkMessage = true
					}
				}
				Expect(foundCreatingVolumesMessage).To(BeTrue(), "Expected to find 'Creating volumes' message")
				Expect(foundCreatingNetworkMessage).To(BeTrue(), "Expected to find 'Creating network' message")

				// Verify container was created and started
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))
				Expect(fakeDockerAPI.ContainerStartCallCount()).To(Equal(1))
			})
		})

		Context("when only one volume exists", func() {
			BeforeEach(func() {
				// Container list starts empty, then returns running container after start
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					if fakeDockerAPI.ContainerStartCallCount() > 0 {
						return []types.Container{
							{
								Names: []string{"/instant-bosh"},
								State: "running",
								Image: "ghcr.io/rkoster/instant-bosh:latest",
							},
						}, nil
					}
					return []types.Container{}, nil
				}

				// One volume exists, one doesn't
				fakeDockerAPI.VolumeInspectStub = func(ctx context.Context, volumeID string) (volume.Volume, error) {
					if volumeID == docker.VolumeStore {
						return volume.Volume{Name: docker.VolumeStore}, nil
					}
					return volume.Volume{}, errdefs.NotFound(errors.New("not found"))
				}

				// Network exists
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{
					Name: docker.NetworkName,
				}, nil)

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:same123"},
				}, nil, nil)

				// Remote has same digest (at latest version)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:same123",
					},
				}, nil)

				// Container inspection returns healthy
				fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							Running: true,
							Health:  &types.Health{Status: "healthy"},
						},
					},
				}, nil)
			})

			It("displays 'Creating volumes' and creates only the missing volume", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).NotTo(HaveOccurred())

				// Verify only one volume was created (the missing one)
				Expect(fakeDockerAPI.VolumeCreateCallCount()).To(Equal(1))

				// Check for "Creating volumes" message (since at least one needs to be created)
				foundCreatingVolumesMessage := false
				foundUsingNetworkMessage := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if strings.Contains(format, "Creating volumes") {
						foundCreatingVolumesMessage = true
					}
					if strings.Contains(format, "Using existing network") {
						foundUsingNetworkMessage = true
					}
				}
				Expect(foundCreatingVolumesMessage).To(BeTrue(), "Expected to find 'Creating volumes' message")
				Expect(foundUsingNetworkMessage).To(BeTrue(), "Expected to find 'Using existing network' message")

				// Verify container was created and started
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(1))
				Expect(fakeDockerAPI.ContainerStartCallCount()).To(Equal(1))
			})
		})

		Context("when volume inspection fails with non-NotFound error", func() {
			BeforeEach(func() {
				// Volumes inspection fails with unexpected error
				fakeDockerAPI.VolumeInspectReturns(volume.Volume{}, errors.New("unexpected error"))

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:same123"},
				}, nil, nil)

				// Remote has same digest (at latest version)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:same123",
					},
				}, nil)
			})

			It("fails fast and returns the error", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check if volume"))

				// Verify volumes were NOT created
				Expect(fakeDockerAPI.VolumeCreateCallCount()).To(Equal(0))

				// Verify container was NOT created
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(0))
			})
		})

		Context("when network inspection fails with non-NotFound error", func() {
			BeforeEach(func() {
				// Volumes exist
				fakeDockerAPI.VolumeInspectReturns(volume.Volume{
					Name: "test-volume",
				}, nil)

				// Network inspection fails with unexpected error
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{}, errors.New("unexpected error"))

				// Image exists locally
				fakeDockerAPI.ImageInspectWithRawReturns(types.ImageInspect{
					ID:          "sha256:local-image-id",
					RepoDigests: []string{"ghcr.io/rkoster/instant-bosh@sha256:same123"},
				}, nil, nil)

				// Remote has same digest (at latest version)
				fakeDockerAPI.DistributionInspectReturns(registry.DistributionInspect{
					Descriptor: ocispec.Descriptor{
						Digest: "sha256:same123",
					},
				}, nil)
			})

			It("fails fast and returns the error", func() {
				err := commands.StartActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider, fakeDirectorFactory, false, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check if network exists"))

				// Verify network was NOT created
				Expect(fakeDockerAPI.NetworkCreateCallCount()).To(Equal(0))

				// Verify container was NOT created
				Expect(fakeDockerAPI.ContainerCreateCallCount()).To(Equal(0))
			})
		})
	})
})
