package commands

const (
	cloudConfigYAML = `azs:
- name: z1
- name: z2
- name: z3

vm_types:
- name: default

disk_types:
- name: default
  disk_size: 1024

networks:
- name: default
  type: manual
  subnets:
  - azs: [z1, z2, z3]
    range: 10.245.0.0/16
    dns: [8.8.8.8]
    # IPs that will not be used for anything
    reserved: [10.245.0.2-10.245.0.10]
    gateway: 10.245.0.1
    static: [10.245.0.34]
    cloud_properties:
      name: instant-bosh

vm_extensions:
- name: all_ports
  cloud_properties:
    ports:
    - 22/tcp

compilation:
  workers: 5
  az: z1
  reuse_compilation_vms: true
  vm_type: default
  network: default
`

	runtimeConfigYAML = `---
# Runtime config to enable SSH on Docker-based VMs
# This ensures systemd starts SSH service on all VMs since it doesn't auto-start in containers

releases:
- name: os-conf
  version: 23.0.0
  url: https://bosh.io/d/github.com/cloudfoundry/os-conf-release?v=23.0.0
  sha1: sha256:efcf30754ce4c5f308aedab3329d8d679f5967b2a4c3c453204c7cb10c7c5ed9

addons:
- name: enable-ssh
  include:
    stemcell:
    - os: ubuntu-noble
  jobs:
  - name: pre-start-script
    release: os-conf
    properties:
      script: |-
        #!/bin/bash
        set -e
        # Start SSH service for bosh ssh to work in Docker containers
        systemctl start ssh || true
`
)

var (
	cloudConfigYAMLBytes   = []byte(cloudConfigYAML)
	runtimeConfigYAMLBytes = []byte(runtimeConfigYAML)
)
