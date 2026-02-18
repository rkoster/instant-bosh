package commands

const (
	// Docker cloud-config - simplified for Docker CPI (no cloud_properties needed for vm_types)
	cloudConfigYAML = `azs:
- name: z1
- name: z2
- name: z3

vm_types:
- name: default
- name: minimal
- name: small
- name: small-highmem
- name: medium
- name: compilation

disk_types:
- name: default
  disk_size: 10240
- name: 10GB
  disk_size: 10240
- name: 100GB
  disk_size: 102400

networks:
- name: default
  type: manual
  subnets:
  - azs: [z1, z2, z3]
    range: 10.245.0.0/16
    dns: [8.8.8.8]
    reserved: [10.245.0.2-10.245.0.10]
    gateway: 10.245.0.1
    static: [10.245.0.34-10.245.0.100]
    cloud_properties:
      name: instant-bosh

vm_extensions:
- name: all_ports
  cloud_properties:
    ports:
    - 22/tcp
- name: 50GB_ephemeral_disk
- name: 100GB_ephemeral_disk
- name: diego-ssh-proxy-network-properties
- name: cf-router-network-properties
- name: cf-tcp-router-network-properties

compilation:
  workers: 5
  az: z1
  reuse_compilation_vms: true
  vm_type: compilation
  network: default
`

	// Incus cloud-config - includes cloud_properties for Incus CPI
	incusCloudConfigYAML = `azs:
- name: z1
- name: z2
- name: z3

vm_types:
- name: default
  cloud_properties:
    instance_type: c2-m4
    ephemeral_disk: 10240
- name: minimal
  cloud_properties:
    instance_type: c1-m4
    ephemeral_disk: 10240
- name: small
  cloud_properties:
    instance_type: c2-m8
    ephemeral_disk: 10240
- name: small-highmem
  cloud_properties:
    instance_type: c2-m10
    ephemeral_disk: 10240
- name: medium
  cloud_properties:
    instance_type: c4-m8
    ephemeral_disk: 10240
- name: compilation
  cloud_properties:
    instance_type: c4-m8
    ephemeral_disk: 51200

disk_types:
- name: default
  disk_size: 10240
- name: 10GB
  disk_size: 10240
- name: 100GB
  disk_size: 102400

networks:
- name: default
  type: manual
  subnets:
  - azs: [z1, z2, z3]
    range: 10.246.0.0/16
    dns: [8.8.8.8]
    gateway: 10.246.0.1
    reserved: [10.246.0.1-10.246.0.20]
    static: [10.246.0.21-10.246.0.100]
    cloud_properties:
      name: instant-bosh-incus

vm_extensions:
- name: 50GB_ephemeral_disk
  cloud_properties:
    ephemeral_disk: 51200
- name: 100GB_ephemeral_disk
  cloud_properties:
    ephemeral_disk: 102400
- name: diego-ssh-proxy-network-properties
  cloud_properties: {}
- name: cf-router-network-properties
  cloud_properties: {}
- name: cf-tcp-router-network-properties
  cloud_properties: {}

compilation:
  workers: 4
  az: z1
  reuse_compilation_vms: true
  vm_type: compilation
  network: default
`
)

var (
	cloudConfigYAMLBytes      = []byte(cloudConfigYAML)
	incusCloudConfigYAMLBytes = []byte(incusCloudConfigYAML)
)

func GetDockerCloudConfigBytes() []byte {
	return cloudConfigYAMLBytes
}

func GetIncusCloudConfigBytes() []byte {
	return incusCloudConfigYAMLBytes
}
