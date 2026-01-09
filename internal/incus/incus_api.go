package incus

import (
	"context"
	"io"

	incus "github.com/lxc/incus/client"
	"github.com/lxc/incus/shared/api"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . IncusAPI
type IncusAPI interface {
	GetServer() (*api.Server, string, error)
	GetInstance(name string) (*api.Instance, string, error)
	GetInstances(instanceType api.InstanceType) ([]api.Instance, error)
	CreateInstance(instance api.InstancesPost) (incus.Operation, error)
	UpdateInstanceState(name string, state api.InstanceStatePut, ETag string) (incus.Operation, error)
	DeleteInstance(name string) (incus.Operation, error)
	GetInstanceFile(instanceName string, filePath string) (io.ReadCloser, *api.InstanceFileResponse, error)
	DeleteInstanceFile(instanceName string, filePath string) error
	CreateInstanceFromImage(source incus.ImageServer, image api.Image, req api.InstancesPost) (incus.RemoteOperation, error)
	
	GetImage(fingerprint string) (*api.Image, string, error)
	GetImageAliases() ([]api.ImageAliasesEntry, error)
	CreateImage(image api.ImagesPost, args *incus.ImageCreateArgs) (incus.Operation, error)
	DeleteImage(fingerprint string) (incus.Operation, error)
	
	GetNetwork(name string) (*api.Network, string, error)
	GetNetworks() ([]api.Network, error)
	CreateNetwork(network api.NetworksPost) error
	DeleteNetwork(name string) (incus.Operation, error)
	
	GetStoragePool(name string) (*api.StoragePool, string, error)
	GetStoragePools() ([]api.StoragePool, error)
	
	GetProfile(name string) (*api.Profile, string, error)
	
	UseProject(name string) incus.InstanceServer
	UseTarget(name string) incus.InstanceServer
	
	GetInstanceLogs(instanceName string) ([]string, error)
	GetInstanceLogfile(instanceName string, filename string) (io.ReadCloser, error)
	
	Disconnect()
}
