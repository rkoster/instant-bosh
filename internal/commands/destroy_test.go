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

var _ = Describe("DestroyAction", func() {
	var (
		fakeCPI *cpifakes.FakeCPI
		fakeUI  *commandsfakes.FakeUI
		logger  boshlog.Logger
	)

	BeforeEach(func() {
		fakeCPI = &cpifakes.FakeCPI{}
		fakeUI = &commandsfakes.FakeUI{}
		logger = boshlog.NewLogger(boshlog.LevelNone)

		// Default: container exists
		fakeCPI.ExistsReturns(true, nil)
		// Default: destroy succeeds
		fakeCPI.DestroyReturns(nil)
	})

	Describe("with force flag", func() {
		Context("when destroying with force=true", func() {
			It("should remove all resources without confirmation", func() {
				err := commands.DestroyAction(fakeUI, logger, fakeCPI, true)

				Expect(err).NotTo(HaveOccurred())

				// Should NOT ask for confirmation
				Expect(fakeUI.AskForConfirmationCallCount()).To(Equal(0))

				// Should check if resources exist
				Expect(fakeCPI.ExistsCallCount()).To(Equal(1))

				// Should destroy resources
				Expect(fakeCPI.DestroyCallCount()).To(Equal(1))

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
				err := commands.DestroyAction(fakeUI, logger, fakeCPI, false)

				Expect(err).NotTo(HaveOccurred())

				// Should ask for confirmation
				Expect(fakeUI.AskForConfirmationCallCount()).To(Equal(1))

				// Should check if resources exist
				Expect(fakeCPI.ExistsCallCount()).To(Equal(1))

				// Should destroy resources
				Expect(fakeCPI.DestroyCallCount()).To(Equal(1))
			})
		})

		Context("when user declines destroy operation", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(errors.New("user cancelled"))
			})

			It("should cancel the operation without removing resources", func() {
				err := commands.DestroyAction(fakeUI, logger, fakeCPI, false)

				Expect(err).NotTo(HaveOccurred())

				// Should ask for confirmation
				Expect(fakeUI.AskForConfirmationCallCount()).To(Equal(1))

				// Should NOT check exists or destroy
				Expect(fakeCPI.ExistsCallCount()).To(Equal(0))
				Expect(fakeCPI.DestroyCallCount()).To(Equal(0))

				// Should print cancellation message
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("error handling", func() {
		Context("when resources don't exist", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(nil)
				fakeCPI.ExistsReturns(false, nil)
			})

			It("should handle missing resources gracefully and complete", func() {
				err := commands.DestroyAction(fakeUI, logger, fakeCPI, false)

				Expect(err).NotTo(HaveOccurred())

				// Should check if resources exist
				Expect(fakeCPI.ExistsCallCount()).To(Equal(1))

				// Should NOT attempt destroy
				Expect(fakeCPI.DestroyCallCount()).To(Equal(0))

				// Should print "no resources found" message
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("when destroy operation fails", func() {
			BeforeEach(func() {
				fakeUI.AskForConfirmationReturns(nil)
				fakeCPI.ExistsReturns(true, nil)
				fakeCPI.DestroyReturns(errors.New("destroy failed"))
			})

			It("should return an error", func() {
				err := commands.DestroyAction(fakeUI, logger, fakeCPI, false)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("destroy failed"))

				// Should attempt destroy
				Expect(fakeCPI.DestroyCallCount()).To(Equal(1))
			})
		})
	})
})
