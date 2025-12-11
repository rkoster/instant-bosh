package commands_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rkoster/instant-bosh/internal/commands"
)

var _ = Describe("Commands", func() {
	Describe("Action Functions", func() {
		It("exports StartAction", func() {
			// Just verify the function exists and has the right signature
			// It will fail when called since Docker isn't available in tests
			fn := commands.StartAction
			Expect(fn).NotTo(BeNil())
		})

		It("exports StopAction", func() {
			fn := commands.StopAction
			Expect(fn).NotTo(BeNil())
		})

		It("exports DestroyAction", func() {
			fn := commands.DestroyAction
			Expect(fn).NotTo(BeNil())
		})

	It("exports EnvAction", func() {
		fn := commands.EnvAction
		Expect(fn).NotTo(BeNil())
	})

		It("exports PrintEnvAction", func() {
			fn := commands.PrintEnvAction
			Expect(fn).NotTo(BeNil())
		})
	})
})
