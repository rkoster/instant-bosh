package commands_test

import (
	"errors"
	"io"
	"strings"

	"github.com/docker/docker/api/types"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("LogsAction", func() {
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

		// Default: container is running
		fakeDockerAPI.ContainerListReturns([]types.Container{
			{
				Names: []string{"/instant-bosh"},
				State: "running",
			},
		}, nil)

		// Default: logs return empty content
		fakeDockerAPI.ContainerLogsReturns(io.NopCloser(strings.NewReader("")), nil)

		// Default: close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Describe("listing components", func() {
		Context("when listComponents flag is true", func() {
			BeforeEach(func() {
				// Return logs with multiple components in Docker multiplexed format
				logContent := `
[main] Starting BOSH director
[director] Director initialized
[uaa] UAA server started
[nats] NATS server running
[main] BOSH ready
`
				fakeDockerAPI.ContainerLogsReturns(io.NopCloser(formatDockerStdout(logContent)), nil)
			})

			It("should list available log components", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, true, nil, false, "")

				Expect(err).NotTo(HaveOccurred())

				// Should request container logs
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(1))

				// Should print available components
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("streaming logs", func() {
		Context("when following logs with no component filter", func() {
			BeforeEach(func() {
				// Return some log content in Docker multiplexed format
				logContent := `[main] Test log message
[director] Another log message`
				fakeDockerAPI.ContainerLogsReturns(io.NopCloser(formatDockerStdout(logContent)), nil)
			})

			It("should stream all logs", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, true, "all")

				Expect(err).NotTo(HaveOccurred())

				// Should request container logs with follow=true
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(1))
			})
		})

		Context("when filtering by specific components", func() {
			BeforeEach(func() {
				logContent := `[main] Main component log
[director] Director component log
[uaa] UAA component log`
				fakeDockerAPI.ContainerLogsReturns(io.NopCloser(formatDockerStdout(logContent)), nil)
			})

			It("should stream logs for specified components only", func() {
				components := []string{"main", "director"}
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, components, false, "100")

				Expect(err).NotTo(HaveOccurred())

				// Should request container logs
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(1))
			})
		})

		Context("when using tail option", func() {
			BeforeEach(func() {
				logContent := strings.Repeat("[main] Log line\n", 200)
				fakeDockerAPI.ContainerLogsReturns(io.NopCloser(formatDockerStdout(logContent)), nil)
			})

			It("should stream last N lines of logs", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, false, "50")

				Expect(err).NotTo(HaveOccurred())

				// Should request container logs
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(1))
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
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, false, "")

				Expect(err).NotTo(HaveOccurred())

				// Should check if container is running
				Expect(fakeDockerAPI.ContainerListCallCount()).To(BeNumerically(">", 0))

				// Should NOT request logs
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
			})
		})

		Context("when container is stopped", func() {
			BeforeEach(func() {
				// Container exists but not running
				fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, true, "")

				Expect(err).NotTo(HaveOccurred())

				// Should NOT request logs
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(0))

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
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, false, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker connection failed"))
			})
		})

		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeDockerAPI.ContainerListReturns(nil, errors.New("docker api error"))
			})

			It("should return an error", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, false, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker api error"))

				// Should NOT request logs if status check fails
				Expect(fakeDockerAPI.ContainerLogsCallCount()).To(Equal(0))
			})
		})

		Context("when fetching logs fails", func() {
			BeforeEach(func() {
				fakeDockerAPI.ContainerLogsReturns(nil, errors.New("log fetch error"))
			})

			It("should return an error when listing components", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, true, nil, false, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("log fetch error"))
			})

			It("should return an error when streaming logs", func() {
				err := commands.LogsActionWithFactory(fakeUI, logger, fakeClientFactory, false, nil, false, "100")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("log fetch error"))
			})
		})
	})
})
