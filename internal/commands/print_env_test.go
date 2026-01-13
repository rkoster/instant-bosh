package commands_test

import (
	"errors"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/cpi/cpifakes"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/director/directorfakes"
)

var _ = Describe("PrintEnvAction", func() {
	var (
		fakeCPI            *cpifakes.FakeCPI
		fakeConfigProvider *directorfakes.FakeConfigProvider
		fakeUI             *commandsfakes.FakeUI
		logger             boshlog.Logger
	)

	BeforeEach(func() {
		fakeCPI = &cpifakes.FakeCPI{}
		fakeConfigProvider = &directorfakes.FakeConfigProvider{}
		fakeUI = &commandsfakes.FakeUI{}
		logger = boshlog.NewLogger(boshlog.LevelNone)

		fakeCPI.IsRunningReturns(true, nil)
		fakeCPI.GetContainerNameReturns("instant-bosh")

		fakeConfigProvider.GetDirectorConfigReturns(&director.Config{
			Environment:  "https://127.0.0.1:25555",
			Client:       "admin",
			ClientSecret: "fake-secret",
			CACert:       "-----BEGIN CERTIFICATE-----\nfake-cert\n-----END CERTIFICATE-----",
			AllProxy:     "ssh+socks5://jumpbox@127.0.0.1:22222?private-key=/tmp/jumpbox-key",
		}, nil)
	})

	Describe("when container is running", func() {
		It("should print environment variables for shell evaluation", func() {
			err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

			Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(1))

			Expect(fakeUI.PrintLinefCallCount()).To(Equal(5))

			format1, args1 := fakeUI.PrintLinefArgsForCall(0)
			Expect(format1).To(Equal("export BOSH_CLIENT=%s"))
			Expect(args1).To(HaveLen(1))
			Expect(args1[0]).To(Equal("admin"))

			format2, args2 := fakeUI.PrintLinefArgsForCall(1)
			Expect(format2).To(Equal("export BOSH_CLIENT_SECRET=%s"))
			Expect(args2).To(HaveLen(1))
			Expect(args2[0]).To(Equal("fake-secret"))

			format3, args3 := fakeUI.PrintLinefArgsForCall(2)
			Expect(format3).To(Equal("export BOSH_ENVIRONMENT=%s"))
			Expect(args3).To(HaveLen(1))
			Expect(args3[0]).To(Equal("https://127.0.0.1:25555"))

			format4, args4 := fakeUI.PrintLinefArgsForCall(3)
			Expect(format4).To(Equal("export BOSH_CA_CERT='%s'"))
			Expect(args4).To(HaveLen(1))
			Expect(args4[0]).To(ContainSubstring("BEGIN CERTIFICATE"))

			format5, args5 := fakeUI.PrintLinefArgsForCall(4)
			Expect(format5).To(Equal("export BOSH_ALL_PROXY=%s"))
			Expect(args5).To(HaveLen(1))
			Expect(args5[0]).To(ContainSubstring("ssh+socks5"))
		})

		It("should output in shell-compatible format", func() {
			err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
				format, _ := fakeUI.PrintLinefArgsForCall(i)
				Expect(format).To(HavePrefix("export "))
			}
		})

		Context("with different director configurations", func() {
			BeforeEach(func() {
				fakeConfigProvider.GetDirectorConfigReturns(&director.Config{
					Environment:  "https://10.0.0.1:25555",
					Client:       "custom-client",
					ClientSecret: "custom-secret",
					CACert:       "custom-cert",
					AllProxy:     "custom-proxy",
				}, nil)
			})

			It("should use the provided configuration", func() {
				err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

				Expect(err).NotTo(HaveOccurred())

				_, args1 := fakeUI.PrintLinefArgsForCall(0)
				Expect(args1[0]).To(Equal("custom-client"))

				_, args2 := fakeUI.PrintLinefArgsForCall(1)
				Expect(args2[0]).To(Equal("custom-secret"))

				_, args3 := fakeUI.PrintLinefArgsForCall(2)
				Expect(args3[0]).To(Equal("https://10.0.0.1:25555"))
			})
		})
	})

	Describe("when container is not running", func() {
		BeforeEach(func() {
			fakeCPI.IsRunningReturns(false, nil)
		})

		It("should return an error instructing to start instant-bosh", func() {
			err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not running"))
			Expect(err.Error()).To(ContainSubstring("ibosh start"))

			Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

			Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(0))

			Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
		})
	})

	Describe("error handling", func() {
		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, errors.New("cpi error"))
			})

			It("should return an error", func() {
				err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cpi error"))

				Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(0))

				Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
			})
		})

		Context("when getting director config fails", func() {
			BeforeEach(func() {
				fakeConfigProvider.GetDirectorConfigReturns(nil, errors.New("config retrieval failed"))
			})

			It("should return an error", func() {
				err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get director config"))
				Expect(err.Error()).To(ContainSubstring("config retrieval failed"))

				Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(1))

				Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
			})
		})
	})

	Describe("usage with eval", func() {
		It("should produce output suitable for shell evaluation", func() {
			err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
				format, args := fakeUI.PrintLinefArgsForCall(i)

				Expect(format).To(ContainSubstring("export"))
				Expect(format).To(ContainSubstring("BOSH_"))

				Expect(args).To(HaveLen(1))
				Expect(args[0]).NotTo(BeEmpty())
			}
		})
	})
})
