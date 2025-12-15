package commands_test

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("DestroyAction", func() {
	var (
		fakeDockerAPI     *dockerfakes.FakeDockerAPI
		fakeClientFactory *dockerfakes.FakeClientFactory
		fakeUI            *commandsfakes.FakeUI
		logger            boshlog.Logger
	)

	BeforeEach(func() {
		fakeDockerAPI = &dockerfakes.FakeDockerAPI{}
		fakeClientFactory = &dockerfakes.FakeClientFactory{}
		fakeUI = &commandsfakes.FakeUI{}

		logger = boshlog.NewLogger(boshlog.LevelNone)

		// Configure fakeClientFactory to return a test client with fakeDockerAPI
		fakeClientFactory.NewClientStub = func(logger boshlog.Logger, customImage string) (*docker.Client, error) {
			return docker.NewTestClient(fakeDockerAPI, logger, docker.ImageName), nil
		}

		// Default: no containers on network
		fakeDockerAPI.NetworkInspectReturns(network.Inspect{
			Containers: make(map[string]network.EndpointResource),
		}, nil)

		// Default: container exists
		fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			// Check if container was removed
			if fakeDockerAPI.ContainerRemoveCallCount() > 0 {
				return []types.Container{}, nil
			}
			return []types.Container{
				{
					Names: []string{"/instant-bosh"},
					State: "running",
				},
			}, nil
		}

		// Default: container inspect succeeds
		fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{}, nil)

		// Default: remove operations succeed
		fakeDockerAPI.ContainerRemoveReturns(nil)
		fakeDockerAPI.VolumeRemoveReturns(nil)
		fakeDockerAPI.NetworkRemoveReturns(nil)

		// Default: close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Describe("with force flag", func() {
		Context("when destroying with force=true", func() {
			It("should remove all resources without confirmation", func() {
				err := commands.DestroyActionWithFactory(fakeUI, logger, fakeClientFactory, true)

				Expect(err).NotTo(HaveOccurred())

				// Should NOT ask for confirmation
				Expect(fakeUI.AskForConfirmationCallCount()).To(Equal(0))

				// Should remove container, volumes, and network
				Expect(fakeDockerAPI.ContainerRemoveCallCount()).To(BeNumerically(">", 0))
				Expect(fakeDockerAPI.VolumeRemoveCallCount()).To(Equal(2)) // store and data volumes
				Expect(fakeDockerAPI.NetworkRemoveCallCount()).To(Equal(1))

				// Should print completion message
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("with confirmation required", func() {
		Context("when user confirms destroy operation", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(nil) // User accepts
			})

			It("should remove all resources after confirmation", func() {
				err := commands.DestroyActionWithFactory(fakeUI, logger, fakeClientFactory, false)

				Expect(err).NotTo(HaveOccurred())

				// Should ask for confirmation
				Expect(fakeUI.AskForConfirmationCallCount()).To(Equal(1))

				// Should remove container, volumes, and network
				Expect(fakeDockerAPI.ContainerRemoveCallCount()).To(BeNumerically(">", 0))
				Expect(fakeDockerAPI.VolumeRemoveCallCount()).To(Equal(2))
				Expect(fakeDockerAPI.NetworkRemoveCallCount()).To(Equal(1))
			})
		})

		Context("when user declines destroy operation", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(errors.New("user cancelled"))
			})

			It("should cancel the operation without removing resources", func() {
				err := commands.DestroyActionWithFactory(fakeUI, logger, fakeClientFactory, false)

				Expect(err).NotTo(HaveOccurred())

				// Should ask for confirmation
				Expect(fakeUI.AskForConfirmationCallCount()).To(Equal(1))

				// Should NOT remove any resources
				Expect(fakeDockerAPI.ContainerRemoveCallCount()).To(Equal(0))
				Expect(fakeDockerAPI.VolumeRemoveCallCount()).To(Equal(0))
				Expect(fakeDockerAPI.NetworkRemoveCallCount()).To(Equal(0))

				// Should print cancellation message
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("with containers on network", func() {
		Context("when other containers exist on instant-bosh network", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(nil)

				// Network has containers
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{
					Containers: map[string]network.EndpointResource{
						"container1": {Name: "zookeeper"},
						"container2": {Name: "instant-bosh"},
					},
				}, nil)
			})

			It("should remove all containers on network except instant-bosh first", func() {
				err := commands.DestroyActionWithFactory(fakeUI, logger, fakeClientFactory, false)

				Expect(err).NotTo(HaveOccurred())

				// Should remove non-instant-bosh containers (zookeeper)
				Expect(fakeDockerAPI.ContainerRemoveCallCount()).To(BeNumerically(">=", 1))

				// Should print messages for each container
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 5))
			})
		})
	})

	Describe("error handling", func() {
		Context("when docker client creation fails", func() {
			BeforeEach(func() {
				fakeClientFactory.NewClientReturns(nil, errors.New("docker connection failed"))
			})

			It("should return an error", func() {
				err := commands.DestroyActionWithFactory(fakeUI, logger, fakeClientFactory, true)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create docker client"))
			})
		})

		Context("when resources don't exist", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(nil)

				// Container doesn't exist
				fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)

				// Network doesn't exist
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{}, errdefs.NotFound(errors.New("not found")))

				// Volume removal fails with not found
				fakeDockerAPI.VolumeRemoveReturns(errdefs.NotFound(errors.New("not found")))

				// Network removal fails with not found
				fakeDockerAPI.NetworkRemoveReturns(errdefs.NotFound(errors.New("not found")))
			})

			It("should handle missing resources gracefully and complete", func() {
				err := commands.DestroyActionWithFactory(fakeUI, logger, fakeClientFactory, false)

				Expect(err).NotTo(HaveOccurred())

				// Should still attempt to remove volumes and network
				Expect(fakeDockerAPI.VolumeRemoveCallCount()).To(Equal(2))
				Expect(fakeDockerAPI.NetworkRemoveCallCount()).To(Equal(1))

				// Should print completion message
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})
	})
})
