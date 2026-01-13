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

var _ = Describe("StopAction", func() {
	var (
		fakeCPI *cpifakes.FakeCPI
		fakeUI  *commandsfakes.FakeUI
		logger  boshlog.Logger
	)

	BeforeEach(func() {
		fakeCPI = &cpifakes.FakeCPI{}
		fakeUI = &commandsfakes.FakeUI{}
		logger = boshlog.NewLogger(boshlog.LevelNone)
	})

	Describe("stopping running container", func() {
		Context("when container is running", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(true, nil)
				fakeCPI.StopReturns(nil)
			})

			It("should stop the container successfully", func() {
				err := commands.StopAction(fakeUI, logger, fakeCPI)

				Expect(err).NotTo(HaveOccurred())

				// Should check if container is running
				Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

				// Should stop the container
				Expect(fakeCPI.StopCallCount()).To(Equal(1))

				// Should print success message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(2))
				Expect(fakeUI.PrintLinefArgsForCall(0)).To(Equal("Stopping instant-bosh container..."))
				Expect(fakeUI.PrintLinefArgsForCall(1)).To(Equal("instant-bosh stopped successfully"))
			})
		})

		Context("when stopping container fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(true, nil)
				fakeCPI.StopReturns(errors.New("failed to stop container"))
			})

			It("should return an error", func() {
				err := commands.StopAction(fakeUI, logger, fakeCPI)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to stop container"))

				// Should attempt to stop the container
				Expect(fakeCPI.StopCallCount()).To(Equal(1))
			})
		})
	})

	Describe("when container is not running", func() {
		Context("when container doesn't exist", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.StopAction(fakeUI, logger, fakeCPI)

				Expect(err).NotTo(HaveOccurred())

				// Should check if container is running
				Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

				// Should NOT attempt to stop
				Expect(fakeCPI.StopCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
				Expect(fakeUI.PrintLinefArgsForCall(0)).To(Equal("instant-bosh is not running"))
			})
		})

		Context("when container exists but is stopped", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, nil)
			})

			It("should inform user and exit gracefully", func() {
				err := commands.StopAction(fakeUI, logger, fakeCPI)

				Expect(err).NotTo(HaveOccurred())

				// Should NOT attempt to stop
				Expect(fakeCPI.StopCallCount()).To(Equal(0))

				// Should print "not running" message
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(1))
				Expect(fakeUI.PrintLinefArgsForCall(0)).To(Equal("instant-bosh is not running"))
			})
		})
	})

	Describe("error handling", func() {
		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, errors.New("cpi error"))
			})

			It("should return an error", func() {
				err := commands.StopAction(fakeUI, logger, fakeCPI)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check if container is running"))

				// Should NOT attempt to stop if status check fails
				Expect(fakeCPI.StopCallCount()).To(Equal(0))
			})
		})
	})
})
