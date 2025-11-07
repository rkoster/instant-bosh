package commands_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rkoster/instant-bosh/internal/commands"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
)

var _ = Describe("Commands", func() {
	var logger boshlog.Logger

	BeforeEach(func() {
		logger = boshlog.NewWriterLogger(boshlog.LevelNone, GinkgoWriter)
	})

	Describe("NewStartCommand", func() {
		It("returns a CLI command with correct name and usage", func() {
			cmd := commands.NewStartCommand(logger)
			Expect(cmd.Name).To(Equal("start"))
			Expect(cmd.Usage).To(Equal("Start instant-bosh director"))
		})
	})

	Describe("NewStopCommand", func() {
		It("returns a CLI command with correct name and usage", func() {
			cmd := commands.NewStopCommand(logger)
			Expect(cmd.Name).To(Equal("stop"))
			Expect(cmd.Usage).To(Equal("Stop instant-bosh director"))
		})
	})

	Describe("NewDestroyCommand", func() {
		It("returns a CLI command with correct name and usage", func() {
			cmd := commands.NewDestroyCommand(logger)
			Expect(cmd.Name).To(Equal("destroy"))
			Expect(cmd.Usage).To(Equal("Destroy instant-bosh and all associated resources"))
		})

		It("includes force flag", func() {
			cmd := commands.NewDestroyCommand(logger)
			Expect(cmd.Flags).To(HaveLen(1))
			Expect(cmd.Flags[0].Names()).To(ContainElement("force"))
		})
	})

	Describe("NewStatusCommand", func() {
		It("returns a CLI command with correct name and usage", func() {
			cmd := commands.NewStatusCommand(logger)
			Expect(cmd.Name).To(Equal("status"))
			Expect(cmd.Usage).To(Equal("Show status of instant-bosh and containers on the network"))
		})
	})

	Describe("NewPullCommand", func() {
		It("returns a CLI command with correct name and usage", func() {
			cmd := commands.NewPullCommand(logger)
			Expect(cmd.Name).To(Equal("pull"))
			Expect(cmd.Usage).To(Equal("Pull the latest instant-bosh image"))
		})
	})
})
