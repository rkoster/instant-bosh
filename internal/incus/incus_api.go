package incus

import (
	"io"

	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . IncusAPI
type IncusAPI interface {
	GetServer() (*api.Server, string, error)
	GetInstance(name string) (*api.Instance, string, error)
	GetInstances(instanceType api.InstanceType) ([]api.Instance, error)
	CreateInstance(instance api.InstancesPost) (incus.Operation, error)
	UpdateInstanceState(name string, state api.InstanceStatePut, ETag string) (incus.Operation, error)
	DeleteInstance(name string) (incus.Operation, error)
	ExecInstance(name string, req api.InstanceExecPost, args *incus.InstanceExecArgs) (incus.Operation, error)
	GetInstanceFile(instanceName string, filePath string) (io.ReadCloser, *incus.InstanceFileResponse, error)
	CreateInstanceFile(instanceName string, filePath string, args incus.InstanceFileArgs) error
	DeleteInstanceFile(instanceName string, filePath string) error
	CreateInstanceFromImage(source incus.ImageServer, image api.Image, req api.InstancesPost) (incus.RemoteOperation, error)
	CopyImage(source incus.ImageServer, image api.Image, args *incus.ImageCopyArgs) (incus.RemoteOperation, error)

	GetImage(fingerprint string) (*api.Image, string, error)
	GetImageAliases() ([]api.ImageAliasesEntry, error)
	CreateImage(image api.ImagesPost, args *incus.ImageCreateArgs) (incus.Operation, error)
	CreateImageAlias(alias api.ImageAliasesPost) error
	DeleteImage(fingerprint string) (incus.Operation, error)

	GetNetwork(name string) (*api.Network, string, error)
	GetNetworks() ([]api.Network, error)
	CreateNetwork(network api.NetworksPost) error
	DeleteNetwork(name string) error

	GetStoragePool(name string) (*api.StoragePool, string, error)
	GetStoragePools() ([]api.StoragePool, error)

	// Storage volume operations
	GetStoragePoolVolume(pool string, volType string, name string) (*api.StorageVolume, string, error)
	CreateStoragePoolVolume(pool string, volume api.StorageVolumesPost) error
	DeleteStoragePoolVolume(pool string, volType string, name string) error

	GetProfile(name string) (*api.Profile, string, error)

	UseProject(name string) incus.InstanceServer
	UseTarget(name string) incus.InstanceServer

	GetInstanceLogfiles(instanceName string) ([]string, error)
	GetInstanceLogfile(instanceName string, filename string) (io.ReadCloser, error)

	Disconnect()
}
