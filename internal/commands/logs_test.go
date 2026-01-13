package commands_test

import (
	"errors"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/cpi/cpifakes"
)

var _ = Describe("LogsAction", func() {
	var (
		fakeCPI *cpifakes.FakeCPI
		fakeUI  *commandsfakes.FakeUI
		logger  boshlog.Logger
	)

	BeforeEach(func() {
		fakeCPI = &cpifakes.FakeCPI{}
		fakeUI = &commandsfakes.FakeUI{}
		logger = boshlog.NewLogger(boshlog.LevelNone)

		// Default: container is running
		fakeCPI.IsRunningReturns(true, nil)

		// Default: logs return empty content
		fakeCPI.GetLogsReturns("", nil)

		// Default: follow logs succeeds
		fakeCPI.FollowLogsWithOptionsReturns(nil)
	})

	Describe("listing components", func() {
		Context("when listComponents flag is true", func() {
			BeforeEach(func() {
				// Return logs with multiple components
				logContent := `
[main] Starting BOSH director
[director] Director initialized
[uaa] UAA server started
[nats] NATS server running
[main] BOSH ready
`
				fakeCPI.GetLogsReturns(logContent, nil)
			})

			It("should list available log components", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, true, nil, false, "")

				Expect(err).NotTo(HaveOccurred())

				// Should check if running
				Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

				// Should request container logs
				Expect(fakeCPI.GetLogsCallCount()).To(Equal(1))

				// Should print available components
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("streaming logs", func() {
		Context("when following logs with no component filter", func() {
			BeforeEach(func() {
				fakeCPI.FollowLogsWithOptionsReturns(nil)
			})

			It("should stream all logs", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, nil, true, "all")

				Expect(err).NotTo(HaveOccurred())

				// Should check if running
				Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

				// Should request container logs with follow=true
				Expect(fakeCPI.FollowLogsWithOptionsCallCount()).To(Equal(1))
				_, follow, tail, _, _ := fakeCPI.FollowLogsWithOptionsArgsForCall(0)
				Expect(follow).To(BeTrue())
				Expect(tail).To(Equal("all"))
			})
		})

		Context("when filtering by specific components", func() {
			BeforeEach(func() {
				fakeCPI.FollowLogsWithOptionsReturns(nil)
			})

			It("should stream logs for specified components only", func() {
				components := []string{"main", "director"}
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, components, false, "100")

				Expect(err).NotTo(HaveOccurred())

				// Should request container logs
				Expect(fakeCPI.FollowLogsWithOptionsCallCount()).To(Equal(1))
				_, follow, tail, _, _ := fakeCPI.FollowLogsWithOptionsArgsForCall(0)
				Expect(follow).To(BeFalse())
				Expect(tail).To(Equal("100"))
			})
		})

		Context("when using tail option", func() {
			BeforeEach(func() {
				fakeCPI.FollowLogsWithOptionsReturns(nil)
			})

			It("should stream last N lines of logs", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, nil, false, "50")

				Expect(err).NotTo(HaveOccurred())

				// Should request container logs
				Expect(fakeCPI.FollowLogsWithOptionsCallCount()).To(Equal(1))
				_, _, tail, _, _ := fakeCPI.FollowLogsWithOptionsArgsForCall(0)
				Expect(tail).To(Equal("50"))
			})
		})
	})

	Describe("when container is not running", func() {
		Context("when container doesn't exist", func() {
			BeforeEach(func() {
				// Container not running
				fakeCPI.IsRunningReturns(false, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, nil, false, "")

				Expect(err).NotTo(HaveOccurred())

				// Should check if container is running
				Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

				// Should NOT request logs
				Expect(fakeCPI.GetLogsCallCount()).To(Equal(0))
				Expect(fakeCPI.FollowLogsWithOptionsCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
			})
		})

		Context("when container is stopped", func() {
			BeforeEach(func() {
				// Container not running
				fakeCPI.IsRunningReturns(false, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, nil, true, "")

				Expect(err).NotTo(HaveOccurred())

				// Should NOT request logs
				Expect(fakeCPI.GetLogsCallCount()).To(Equal(0))
				Expect(fakeCPI.FollowLogsWithOptionsCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
			})
		})
	})

	Describe("error handling", func() {
		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, errors.New("cpi error"))
			})

			It("should return an error", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, nil, false, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cpi error"))

				// Should NOT request logs if status check fails
				Expect(fakeCPI.GetLogsCallCount()).To(Equal(0))
				Expect(fakeCPI.FollowLogsWithOptionsCallCount()).To(Equal(0))
			})
		})

		Context("when fetching logs fails", func() {
			BeforeEach(func() {
				fakeCPI.GetLogsReturns("", errors.New("log fetch error"))
			})

			It("should return an error when listing components", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, true, nil, false, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("log fetch error"))
			})
		})

		Context("when streaming logs fails", func() {
			BeforeEach(func() {
				fakeCPI.FollowLogsWithOptionsReturns(errors.New("log stream error"))
			})

			It("should return an error when streaming logs", func() {
				err := commands.LogsAction(fakeUI, logger, fakeCPI, false, nil, false, "100")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("log stream error"))
			})
		})
	})
})
