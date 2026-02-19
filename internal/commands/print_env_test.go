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
			Environment:        "https://127.0.0.1:25555",
			Client:             "admin",
			ClientSecret:       "fake-secret",
			CACert:             "-----BEGIN CERTIFICATE-----\nfake-cert\n-----END CERTIFICATE-----",
			AllProxy:           "ssh+socks5://jumpbox@127.0.0.1:22222?private-key=/tmp/jumpbox-key",
			ConfigServerURL:    "https://127.0.0.1:8081",
			ConfigServerClient: "director_config_server",
			ConfigServerSecret: "fake-config-server-secret",
			ConfigServerCACert: "-----BEGIN CERTIFICATE-----\nfake-config-server-cert\n-----END CERTIFICATE-----",
			UAAURL:             "https://127.0.0.1:8443",
			UAACACert:          "-----BEGIN CERTIFICATE-----\nfake-uaa-cert\n-----END CERTIFICATE-----",
		}, nil)
	})

	Describe("when container is running", func() {
		It("should print environment variables for shell evaluation", func() {
			err := commands.PrintEnvAction(fakeUI, logger, fakeCPI, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))

			Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(1))

			Expect(fakeUI.PrintLinefCallCount()).To(Equal(11))

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

			format6, args6 := fakeUI.PrintLinefArgsForCall(5)
			Expect(format6).To(Equal("export CONFIG_SERVER_URL=%s"))
			Expect(args6).To(HaveLen(1))
			Expect(args6[0]).To(Equal("https://127.0.0.1:8081"))

			format7, args7 := fakeUI.PrintLinefArgsForCall(6)
			Expect(format7).To(Equal("export CONFIG_SERVER_CLIENT=%s"))
			Expect(args7).To(HaveLen(1))
			Expect(args7[0]).To(Equal("director_config_server"))

			format8, args8 := fakeUI.PrintLinefArgsForCall(7)
			Expect(format8).To(Equal("export CONFIG_SERVER_SECRET=%s"))
			Expect(args8).To(HaveLen(1))
			Expect(args8[0]).To(Equal("fake-config-server-secret"))

			format9, args9 := fakeUI.PrintLinefArgsForCall(8)
			Expect(format9).To(Equal("export CONFIG_SERVER_CA_CERT='%s'"))
			Expect(args9).To(HaveLen(1))
			Expect(args9[0]).To(ContainSubstring("BEGIN CERTIFICATE"))

			format10, args10 := fakeUI.PrintLinefArgsForCall(9)
			Expect(format10).To(Equal("export UAA_URL=%s"))
			Expect(args10).To(HaveLen(1))
			Expect(args10[0]).To(Equal("https://127.0.0.1:8443"))

			format11, args11 := fakeUI.PrintLinefArgsForCall(10)
			Expect(format11).To(Equal("export UAA_CA_CERT='%s'"))
			Expect(args11).To(HaveLen(1))
			Expect(args11[0]).To(ContainSubstring("BEGIN CERTIFICATE"))
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
					Environment:        "https://10.0.0.1:25555",
					Client:             "custom-client",
					ClientSecret:       "custom-secret",
					CACert:             "custom-cert",
					AllProxy:           "custom-proxy",
					ConfigServerURL:    "https://10.0.0.1:8081",
					ConfigServerClient: "custom-config-client",
					ConfigServerSecret: "custom-config-secret",
					ConfigServerCACert: "custom-config-cert",
					UAAURL:             "https://10.0.0.1:8443",
					UAACACert:          "custom-uaa-cert",
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
				Expect(format).To(SatisfyAny(
					ContainSubstring("BOSH_"),
					ContainSubstring("CONFIG_SERVER_"),
					ContainSubstring("UAA_"),
				))

				Expect(args).To(HaveLen(1))
				Expect(args[0]).NotTo(BeEmpty())
			}
		})
	})
})
