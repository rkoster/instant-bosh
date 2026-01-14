package commands_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/cpi/cpifakes"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/director/directorfakes"
)

var _ = Describe("StartAction", func() {
	var (
		fakeCPI             *cpifakes.FakeCPI
		fakeConfigProvider  *directorfakes.FakeConfigProvider
		fakeDirectorFactory *directorfakes.FakeDirectorFactory
		fakeDirector        *directorfakes.FakeDirector
		fakeUI              *commandsfakes.FakeUI
		logger              boshlog.Logger
		opts                cpi.StartOptions
	)

	BeforeEach(func() {
		fakeCPI = &cpifakes.FakeCPI{}
		fakeConfigProvider = &directorfakes.FakeConfigProvider{}
		fakeDirectorFactory = &directorfakes.FakeDirectorFactory{}
		fakeDirector = &directorfakes.FakeDirector{}
		fakeUI = &commandsfakes.FakeUI{}

		logger = boshlog.NewLogger(boshlog.LevelNone)
		opts = cpi.StartOptions{
			SkipUpdate:         false,
			SkipStemcellUpload: true,
			CustomImage:        "",
		}

		fakeConfigProvider.GetDirectorConfigReturns(&director.Config{
			Environment:  "https://127.0.0.1:25555",
			Client:       "admin",
			ClientSecret: "fake-password",
			CACert:       "fake-cert",
		}, nil)

		fakeDirectorFactory.NewDirectorReturns(fakeDirector, nil)
		fakeDirector.UpdateCloudConfigReturns(nil)
		fakeUI.AskForConfirmationReturns(nil)

		fakeCPI.GetContainerNameReturns("instant-bosh")
		fakeCPI.GetHostAddressReturns("127.0.0.1")
		fakeCPI.GetCloudConfigBytesReturns([]byte("cloud-config: test"))

		fakeCPI.IsRunningReturns(false, nil)
		fakeCPI.ExistsReturns(false, nil)

		fakeCPI.StartReturns(nil)
		fakeCPI.EnsurePrerequisitesReturns(nil)
		fakeCPI.WaitForReadyReturns(nil)

		fakeCPI.FollowLogsWithOptionsStub = func(ctx context.Context, follow bool, tail string, stdout, stderr io.Writer) error {
			return nil
		}

		fakeDirector.StemcellsReturns([]boshdir.Stemcell{}, nil)
		fakeDirector.UploadStemcellFileReturns(nil)
	})

	Describe("fresh start scenario", func() {
		Context("when no container exists", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, nil)
				fakeCPI.ExistsReturns(false, nil)
			})

			It("starts the container successfully", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeCPI.EnsurePrerequisitesCallCount()).To(Equal(1))
				Expect(fakeCPI.IsRunningCallCount()).To(Equal(1))
				Expect(fakeCPI.ExistsCallCount()).To(Equal(1))
				Expect(fakeCPI.StartCallCount()).To(Equal(1))
				Expect(fakeCPI.WaitForReadyCallCount()).To(Equal(1))

				_, timeout := fakeCPI.WaitForReadyArgsForCall(0)
				Expect(timeout).To(Equal(5 * time.Minute))

				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))
				Expect(fakeUI.PrintLinefCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("when container exists but is not running", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, nil)
				fakeCPI.ExistsReturns(true, nil)
				fakeCPI.DestroyReturns(nil)
			})

			It("removes stopped container and starts a new one", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeCPI.DestroyCallCount()).To(Equal(1))
				Expect(fakeCPI.StartCallCount()).To(Equal(1))
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))
			})
		})
	})

	Describe("container already running scenarios", func() {
		Context("when container is already running", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(true, nil)
			})

			It("displays already running message without recreating container", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeCPI.DestroyCallCount()).To(Equal(0))
				Expect(fakeCPI.StartCallCount()).To(Equal(0))
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(0))

				foundAlreadyRunning := false
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					if format == "instant-bosh is already running" {
						foundAlreadyRunning = true
						break
					}
				}
				Expect(foundAlreadyRunning).To(BeTrue(), "Expected to find 'already running' message")
			})
		})
	})

	Describe("error handling", func() {
		Context("when EnsurePrerequisites fails", func() {
			BeforeEach(func() {
				fakeCPI.EnsurePrerequisitesReturns(errors.New("prerequisites failed"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure prerequisites"))
				Expect(fakeCPI.StartCallCount()).To(Equal(0))
			})
		})

		Context("when IsRunning check fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, errors.New("status check failed"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("status check failed"))
				Expect(fakeCPI.StartCallCount()).To(Equal(0))
			})
		})

		Context("when Exists check fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, nil)
				fakeCPI.ExistsReturns(false, errors.New("exists check failed"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("exists check failed"))
				Expect(fakeCPI.StartCallCount()).To(Equal(0))
			})
		})

		Context("when destroying stopped container fails", func() {
			BeforeEach(func() {
				fakeCPI.IsRunningReturns(false, nil)
				fakeCPI.ExistsReturns(true, nil)
				fakeCPI.DestroyReturns(errors.New("destroy failed"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to remove stopped container"))
				Expect(fakeCPI.StartCallCount()).To(Equal(0))
			})
		})

		Context("when Start fails", func() {
			BeforeEach(func() {
				fakeCPI.StartReturns(errors.New("start failed"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to start container"))
				Expect(fakeCPI.WaitForReadyCallCount()).To(Equal(0))
			})
		})

		Context("when WaitForReady fails", func() {
			BeforeEach(func() {
				fakeCPI.WaitForReadyReturns(errors.New("not ready"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("BOSH failed to become ready"))
				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(0))
			})
		})

		Context("when applying cloud-config fails", func() {
			BeforeEach(func() {
				fakeDirector.UpdateCloudConfigReturns(errors.New("cloud-config update failed"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to apply cloud-config"))
			})
		})

		Context("when getting director config fails", func() {
			BeforeEach(func() {
				fakeConfigProvider.GetDirectorConfigReturns(nil, errors.New("config provider error"))
			})

			It("returns an error", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("config provider error"))
			})
		})
	})

	Describe("cloud-config application", func() {
		Context("when cloud-config application succeeds", func() {
			BeforeEach(func() {
				fakeCPI.GetCloudConfigBytesReturns([]byte("cloud_provider:\n  name: docker\n"))
			})

			It("applies the cloud-config using CPI's configuration", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))
				Expect(fakeCPI.GetCloudConfigBytesCallCount()).To(Equal(1))
			})
		})
	})

	Describe("stemcell upload behavior", func() {
		Context("when skip-stemcell-upload flag is set", func() {
			BeforeEach(func() {
				opts.SkipStemcellUpload = true
			})

			It("does not upload stemcells", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeDirector.StemcellsCallCount()).To(Equal(0))
				Expect(fakeDirector.UploadStemcellFileCallCount()).To(Equal(0))
			})
		})

		Context("when skip-stemcell-upload flag is not set with non-Docker CPI", func() {
			BeforeEach(func() {
				opts.SkipStemcellUpload = false
			})

			It("skips stemcell upload since fake CPI is not Docker-based", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("log streaming", func() {
		Context("when FollowLogsWithOptions is called", func() {
			var logStreamCalled bool
			var logStreamMutex sync.Mutex

			BeforeEach(func() {
				logStreamCalled = false
				fakeCPI.FollowLogsWithOptionsStub = func(ctx context.Context, follow bool, tail string, stdout, stderr io.Writer) error {
					logStreamMutex.Lock()
					logStreamCalled = true
					logStreamMutex.Unlock()
					Expect(follow).To(BeTrue())
					Expect(tail).To(Equal("all"))
					return nil
				}
			})

			It("streams logs during startup", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					logStreamMutex.Lock()
					defer logStreamMutex.Unlock()
					return logStreamCalled
				}, "1s").Should(BeTrue())
			})
		})
	})

	Describe("UI messages", func() {
		Context("when starting successfully", func() {
			It("displays appropriate progress messages", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				messages := []string{}
				for i := 0; i < fakeUI.PrintLinefCallCount(); i++ {
					format, _ := fakeUI.PrintLinefArgsForCall(i)
					messages = append(messages, format)
				}

				Expect(messages).To(ContainElement(ContainSubstring("Starting instant-bosh container")))
				Expect(messages).To(ContainElement(ContainSubstring("Waiting for BOSH to be ready")))
				Expect(messages).To(ContainElement(ContainSubstring("instant-bosh is ready")))
				Expect(messages).To(ContainElement(ContainSubstring("Applying cloud-config")))
			})
		})
	})

	Describe("integration with director", func() {
		Context("when director factory and config provider work together", func() {
			It("successfully creates director and applies configuration", func() {
				err := commands.StartAction(fakeUI, logger, fakeCPI, fakeConfigProvider, fakeDirectorFactory, opts)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeConfigProvider.GetDirectorConfigCallCount()).To(Equal(1))

				Expect(fakeDirectorFactory.NewDirectorCallCount()).To(Equal(1))
				config, _ := fakeDirectorFactory.NewDirectorArgsForCall(0)
				Expect(config).NotTo(BeNil())
				Expect(config.Environment).To(Equal("https://127.0.0.1:25555"))

				Expect(fakeDirector.UpdateCloudConfigCallCount()).To(Equal(1))
			})
		})
	})
})
