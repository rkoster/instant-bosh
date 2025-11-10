package docker_test

import (
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
