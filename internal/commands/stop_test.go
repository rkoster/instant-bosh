package commands_test

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("StopAction", func() {
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

		// Default: container stop succeeds
		fakeDockerAPI.ContainerStopReturns(nil)

		// Default: close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Describe("stopping running container", func() {
		Context("when container is running", func() {
			BeforeEach(func() {
				// Container is running
				fakeDockerAPI.ContainerListStub = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
					// After stop: container is not running
					if fakeDockerAPI.ContainerStopCallCount() > 0 {
						return []types.Container{}, nil
					}
					// Initially: container is running
					return []types.Container{
						{
							Names: []string{"/instant-bosh"},
							State: "running",
						},
					}, nil
				}
			})

			It("should stop the container successfully", func() {
				err := commands.StopActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).NotTo(HaveOccurred())

				// Should check if container is running
				Expect(fakeDockerAPI.ContainerListCallCount()).To(BeNumerically(">", 0))

				// Should stop the container
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(1))

				// Should print success message
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("when stopping container fails", func() {
			BeforeEach(func() {
				// Container is running
				fakeDockerAPI.ContainerListReturns([]types.Container{
					{
						Names: []string{"/instant-bosh"},
						State: "running",
					},
				}, nil)

				// Stop operation fails
				fakeDockerAPI.ContainerStopReturns(errors.New("failed to stop container"))
			})

			It("should return an error", func() {
				err := commands.StopActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to stop container"))

				// Should attempt to stop the container
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(1))
			})
		})
	})

	Describe("when container is not running", func() {
		Context("when container doesn't exist", func() {
			BeforeEach(func() {
				// No containers running
				fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.StopActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).NotTo(HaveOccurred())

				// Should check if container is running
				Expect(fakeDockerAPI.ContainerListCallCount()).To(BeNumerically(">", 0))

				// Should NOT attempt to stop
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
			})
		})

		Context("when container exists but is stopped", func() {
			BeforeEach(func() {
				// Container exists but not running (stopped state)
				fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.StopActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).NotTo(HaveOccurred())

				// Should NOT attempt to stop
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
			})
		})
	})

	Describe("error handling", func() {
		Context("when docker client creation fails", func() {
			BeforeEach(func() {
				fakeClientFactory.NewClientReturns(nil, errors.New("docker connection failed"))
			})

			It("should return an error", func() {
				err := commands.StopActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create docker client"))
			})
		})

		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeDockerAPI.ContainerListReturns(nil, errors.New("docker api error"))
			})

			It("should return an error", func() {
				err := commands.StopActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker api error"))

				// Should NOT attempt to stop if status check fails
				Expect(fakeDockerAPI.ContainerStopCallCount()).To(Equal(0))
			})
		})
	})
})
