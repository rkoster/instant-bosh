package docker_test

import (
	"os"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rkoster/instant-bosh/internal/docker"
)

var _ = Describe("Docker Client Constants", func() {
	Describe("Constants", func() {
		It("defines the correct container name", func() {
			Expect(docker.ContainerName).To(Equal("instant-bosh"))
		})

		It("defines the correct network name", func() {
			Expect(docker.NetworkName).To(Equal("instant-bosh"))
		})

		It("defines the correct volume names", func() {
			Expect(docker.VolumeStore).To(Equal("instant-bosh-store"))
			Expect(docker.VolumeData).To(Equal("instant-bosh-data"))
		})

		It("defines the correct image name", func() {
			Expect(docker.ImageName).To(Equal("ghcr.io/rkoster/instant-bosh:latest"))
		})

		It("defines the correct network configuration", func() {
			Expect(docker.NetworkSubnet).To(Equal("10.245.0.0/16"))
			Expect(docker.NetworkGateway).To(Equal("10.245.0.1"))
			Expect(docker.ContainerIP).To(Equal("10.245.0.10"))
		})

		It("defines the correct port mappings", func() {
			Expect(docker.DirectorPort).To(Equal("25555"))
			Expect(docker.SSHPort).To(Equal("2222"))
		})
	})
})

var _ = Describe("Docker Client", func() {
	var (
		logger         boshlog.Logger
		originalHost   string
		hostWasSet     bool
	)

	BeforeEach(func() {
		logger = boshlog.NewLogger(boshlog.LevelNone)
		originalHost, hostWasSet = os.LookupEnv("DOCKER_HOST")
	})

	AfterEach(func() {
		if hostWasSet {
			os.Setenv("DOCKER_HOST", originalHost)
		} else {
			os.Unsetenv("DOCKER_HOST")
		}
	})

	Describe("NewClient", func() {
		Context("when DOCKER_HOST is set to a unix socket", func() {
			It("respects the docker context socket location", func() {
				// Set DOCKER_HOST to simulate a custom Docker context (like Colima)
				customSocket := "unix:///Users/test/.config/colima/default/docker.sock"
				os.Setenv("DOCKER_HOST", customSocket)

				client, err := docker.NewClient(logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).NotTo(BeNil())
				defer client.Close()

				// We can't directly access the socketPath field as it's private,
				// but the test verifies that the client was created successfully
				// with the custom DOCKER_HOST. The actual socket path usage is
				// tested through integration tests.
			})
		})

		Context("when DOCKER_HOST is not set", func() {
			It("creates a client with the default socket", func() {
				os.Unsetenv("DOCKER_HOST")

				client, err := docker.NewClient(logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).NotTo(BeNil())
				defer client.Close()
			})
		})
	})
})
