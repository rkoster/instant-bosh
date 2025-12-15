package commands_test

import (
	"errors"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("EnvAction", func() {
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

		// Default: network with containers
		fakeDockerAPI.NetworkInspectReturns(network.Inspect{
			Containers: map[string]network.EndpointResource{
				"instant-bosh-id": {Name: "instant-bosh"},
			},
		}, nil)

		// Default: close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Describe("when container is running", func() {
		BeforeEach(func() {
			// Container is running
			fakeDockerAPI.ContainerListReturns([]types.Container{
				{
					Names:   []string{"/instant-bosh"},
					State:   "running",
					ID:      "instant-bosh-id",
					Created: time.Now().Add(-1 * time.Hour).Unix(),
				},
			}, nil)

			// Container inspect returns detailed info
			fakeDockerAPI.ContainerInspectReturns(types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:      "instant-bosh-id",
					Name:    "/instant-bosh",
					Created: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				},
			}, nil)

			// For simplicity, mock exec to return error (release fetch is optional)
			fakeDockerAPI.ContainerExecCreateReturns(types.IDResponse{}, errors.New("exec not mocked"))
		})

		It("should display environment information", func() {
			err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

			Expect(err).NotTo(HaveOccurred())

			// Should check if container is running
			Expect(fakeDockerAPI.ContainerListCallCount()).To(BeNumerically(">", 0))

			// Should print environment details (name, state, IP, ports)
			Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 3))

			// Should print table (containers)
			Expect(fakeUI.PrintTableCallCount()).To(BeNumerically(">", 0))
		})

		It("should handle release fetch errors gracefully", func() {
			err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

			Expect(err).NotTo(HaveOccurred())

			// Should still complete even if release fetch fails
			Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 3))
		})

		It("should display containers on network", func() {
			// Add more containers to network
			fakeDockerAPI.NetworkInspectReturns(network.Inspect{
				Containers: map[string]network.EndpointResource{
					"instant-bosh-id": {Name: "instant-bosh"},
					"zookeeper-id":    {Name: "zookeeper"},
				},
			}, nil)

			err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

			Expect(err).NotTo(HaveOccurred())

			// Should print containers table
			Expect(fakeUI.PrintTableCallCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("when container is stopped", func() {
		BeforeEach(func() {
			// Container not running
			fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)

			// But container exists on network
			fakeDockerAPI.NetworkInspectReturns(network.Inspect{
				Containers: map[string]network.EndpointResource{
					"instant-bosh-id": {Name: "instant-bosh"},
				},
			}, nil)
		})

		It("should display stopped state without IP and ports", func() {
			err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

			Expect(err).NotTo(HaveOccurred())

			// Should print environment name and stopped state
			Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 2))

			// Should NOT fetch releases (container not running)
			Expect(fakeDockerAPI.ContainerExecCreateCallCount()).To(Equal(0))

			// Should still print containers table
			Expect(fakeUI.PrintTableCallCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("error handling", func() {
		Context("when docker client creation fails", func() {
			BeforeEach(func() {
				fakeClientFactory.NewClientReturns(nil, errors.New("docker connection failed"))
			})

			It("should return an error", func() {
				err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create docker client"))
			})
		})

		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeDockerAPI.ContainerListReturns(nil, errors.New("docker api error"))
			})

			It("should return an error", func() {
				err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker api error"))
			})
		})

		Context("when network inspection fails", func() {
			BeforeEach(func() {
				// Container is running
				fakeDockerAPI.ContainerListReturns([]types.Container{
					{Names: []string{"/instant-bosh"}, State: "running"},
				}, nil)

				// Network inspection fails
				fakeDockerAPI.NetworkInspectReturns(network.Inspect{}, errors.New("network error"))
			})

			It("should handle error gracefully and continue", func() {
				err := commands.EnvActionWithFactory(fakeUI, logger, fakeClientFactory)

				Expect(err).NotTo(HaveOccurred())

				// Should still print environment information
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 3))
			})
		})
	})
})
