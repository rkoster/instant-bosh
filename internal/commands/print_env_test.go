package commands_test

import (
	"errors"

	"github.com/docker/docker/api/types"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/director/directorfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("PrintEnvAction", func() {
	var (
		fakeDockerAPI      *dockerfakes.FakeDockerAPI
		fakeClientFactory  *dockerfakes.FakeClientFactory
		fakeConfigProvider *directorfakes.FakeConfigProvider
		fakeUI             *commandsfakes.FakeUI
		logger             boshlog.Logger
	)

	BeforeEach(func() {
		fakeDockerAPI = &dockerfakes.FakeDockerAPI{}
		fakeClientFactory = &dockerfakes.FakeClientFactory{}
		fakeConfigProvider = &directorfakes.FakeConfigProvider{}
		fakeUI = &commandsfakes.FakeUI{}

		logger = boshlog.NewLogger(boshlog.LevelNone)

		// Configure fakeClientFactory to return a test client with fakeDockerAPI
		fakeClientFactory.NewClientStub = func(logger boshlog.Logger, customImage string) (*docker.Client, error) {
			return docker.NewTestClient(fakeDockerAPI, logger, docker.ImageName), nil
		}

		// Configure fakeConfigProvider to return director config
		fakeConfigProvider.GetDirectorConfigReturns(&director.Config{
			Environment:  "https://127.0.0.1:25555",
			Client:       "admin",
			ClientSecret: "fake-secret",
			CACert:       "-----BEGIN CERTIFICATE-----\nfake-cert\n-----END CERTIFICATE-----",
			AllProxy:     "ssh+socks5://jumpbox@127.0.0.1:22222?private-key=/tmp/jumpbox-key",
		}, nil)

		// Default: container is running
		fakeDockerAPI.ContainerListReturns([]types.Container{
			{
				Names: []string{"/instant-bosh"},
				State: "running",
			},
		}, nil)

		// Default: close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Describe("when container is running", func() {
		It("should print environment variables for shell evaluation", func() {
			err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			// Should check if container is running
			Expect(fakeDockerAPI.ContainerListCallCount()).To(BeNumerically(">", 0))

			// Should get director config
			Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(1))

			// Should print all required environment variables
			Expect(fakeUI.PrintLinefCallCount()).To(Equal(5))

			// Verify the environment variables are printed correctly
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
			err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			// All printed lines should start with "export "
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
				err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

				Expect(err).NotTo(HaveOccurred())

				// Should print custom configuration values
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
			// No containers running
			fakeDockerAPI.ContainerListReturns([]types.Container{}, nil)
		})

		It("should return an error instructing to start instant-bosh", func() {
			err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("instant-bosh container is not running"))
			Expect(err.Error()).To(ContainSubstring("ibosh start"))

			// Should check container status
			Expect(fakeDockerAPI.ContainerListCallCount()).To(BeNumerically(">", 0))

			// Should NOT get director config
			Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(0))

			// Should NOT print environment variables
			Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
		})
	})

	Describe("error handling", func() {
		Context("when docker client creation fails", func() {
			BeforeEach(func() {
				fakeClientFactory.NewClientReturns(nil, errors.New("docker connection failed"))
			})

			It("should return an error", func() {
				err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create docker client"))

				// Should NOT print environment variables
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
			})
		})

		Context("when checking container status fails", func() {
			BeforeEach(func() {
				fakeDockerAPI.ContainerListReturns(nil, errors.New("docker api error"))
			})

			It("should return an error", func() {
				err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker api error"))

				// Should NOT get director config
				Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(0))

				// Should NOT print environment variables
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
			})
		})

		Context("when getting director config fails", func() {
			BeforeEach(func() {
				fakeConfigProvider.GetDirectorConfigReturns(nil, errors.New("config retrieval failed"))
			})

			It("should return an error", func() {
				err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get director config"))
				Expect(err.Error()).To(ContainSubstring("config retrieval failed"))

				// Should attempt to get director config
				Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(1))

				// Should NOT print environment variables
				Expect(fakeUI.PrintLinefCallCount()).To(Equal(0))
			})
		})
	})

	Describe("usage with eval", func() {
		It("should produce output suitable for shell evaluation", func() {
			err := commands.PrintEnvActionWithFactories(fakeUI, logger, fakeClientFactory, fakeConfigProvider)

			Expect(err).NotTo(HaveOccurred())

			// All output should be valid shell export statements
			// Format: export VAR_NAME=value
			for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
				format, args := fakeUI.PrintLinefArgsForCall(i)

				// Should be an export statement
				Expect(format).To(ContainSubstring("export"))
				Expect(format).To(ContainSubstring("BOSH_"))

				// Should have at least one argument (the value)
				Expect(args).To(HaveLen(1))
				Expect(args[0]).NotTo(BeEmpty())
			}
		})
	})
})
