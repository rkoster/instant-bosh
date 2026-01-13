package commands_test

import (
	"errors"
	"time"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/cpi/cpifakes"
)

var _ = Describe("EnvAction", func() {
	var (
		fakeCPI *cpifakes.FakeCPI
		fakeUI  *commandsfakes.FakeUI
		logger  boshlog.Logger
	)

	BeforeEach(func() {
		fakeCPI = &cpifakes.FakeCPI{}
		fakeUI = &commandsfakes.FakeUI{}
		logger = boshlog.NewLogger(boshlog.LevelNone)

		fakeCPI.GetContainerNameReturns("instant-bosh")
		fakeCPI.GetContainersOnNetworkReturns([]cpi.ContainerInfo{
			{
				Name:    "instant-bosh",
				Created: time.Now().Add(-1 * time.Hour),
				Network: "instant-bosh-network",
			},
		}, nil)
	})

	Describe("when container is running", func() {
		BeforeEach(func() {
			fakeCPI.IsRunningReturns(true, nil)
			fakeCPI.ExecCommandReturns("", errors.New("exec not mocked"))
		})

		It("should display environment information", func() {
			err := commands.EnvAction(fakeUI, logger, fakeCPI)

			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

			Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 3))

			Expect(fakeUI.PrintTableCallCount()).To(BeNumerically(">", 0))
		})

		It("should handle release fetch errors gracefully", func() {
			err := commands.EnvAction(fakeUI, logger, fakeCPI)

			Expect(err).NotTo(HaveOccurred())

			Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 3))
		})

		It("should display containers on network", func() {
			fakeCPI.GetContainersOnNetworkReturns([]cpi.ContainerInfo{
				{Name: "instant-bosh", Created: time.Now().Add(-1 * time.Hour), Network: "instant-bosh-network"},
				{Name: "zookeeper", Created: time.Now().Add(-30 * time.Minute), Network: "instant-bosh-network"},
			}, nil)

			err := commands.EnvAction(fakeUI, logger, fakeCPI)

			Expect(err).NotTo(HaveOccurred())

			Expect(fakeUI.PrintTableCallCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("when container is stopped", func() {
		BeforeEach(func() {
			fakeCPI.IsRunningReturns(false, nil)
		})

		It("should display stopped state without IP and ports", func() {
			err := commands.EnvAction(fakeUI, logger, fakeCPI)

			Expect(err).NotTo(HaveOccurred())

			Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 2))

			Expect(fakeCPI.ExecCommandCallCount()).To(Equal(0))

			Expect(fakeUI.PrintTableCallCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("error handling", func() {
		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, errors.New("cpi error"))
			})

			It("should return an error", func() {
				err := commands.EnvAction(fakeUI, logger, fakeCPI)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cpi error"))
			})
		})

		Context("when getting containers on network fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(true, nil)
				fakeCPI.ExecCommandReturns("", errors.New("exec failed"))
				fakeCPI.GetContainersOnNetworkReturns(nil, errors.New("network error"))
			})

			It("should handle error gracefully and continue", func() {
				err := commands.EnvAction(fakeUI, logger, fakeCPI)

				Expect(err).NotTo(HaveOccurred())

				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 3))
			})
		})
	})
})
