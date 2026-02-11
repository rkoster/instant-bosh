package incus

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/gorilla/websocket"
	incusclient "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
	"github.com/stretchr/testify/require"
)

type fakeOperation struct{}

type fakeRemoteOperation struct{}

func (fakeOperation) AddHandler(func(api.Operation)) (*incusclient.EventTarget, error) {
	return nil, nil
}
func (fakeOperation) Cancel() error                                { return nil }
func (fakeOperation) Get() api.Operation                           { return api.Operation{} }
func (fakeOperation) GetWebsocket(string) (*websocket.Conn, error) { return nil, nil }
func (fakeOperation) RemoveHandler(*incusclient.EventTarget) error { return nil }
func (fakeOperation) Refresh() error                               { return nil }
func (fakeOperation) Wait() error                                  { return nil }
func (fakeOperation) WaitContext(context.Context) error            { return nil }

func (fakeRemoteOperation) AddHandler(func(api.Operation)) (*incusclient.EventTarget, error) {
	return nil, nil
}
func (fakeRemoteOperation) CancelTarget() error                { return nil }
func (fakeRemoteOperation) GetTarget() (*api.Operation, error) { return nil, nil }
func (fakeRemoteOperation) Wait() error                        { return nil }

type fakeIncusAPI struct {
	getStoragePoolVolumeResults      []storageVolumeResult
	getStoragePoolVolumeCallCount    int
	createStoragePoolVolumeErr       error
	createStoragePoolVolumeCallCount int
	createStoragePoolVolumeArgs      []storageVolumeCreateArgs
	createInstanceArgs               []api.InstancesPost
	createInstanceOp                 incusclient.Operation
	createInstanceErr                error
	createInstanceFromImageOp        incusclient.RemoteOperation
	createInstanceFromImageErr       error
	createInstanceFileErr            error
	deleteInstanceFileErr            error
	updateInstanceStateOp            incusclient.Operation
	updateInstanceStateErr           error
	execInstanceOp                   incusclient.Operation
	execInstanceErr                  error
	getImageResult                   *api.Image
	getImageErr                      error
}

type storageVolumeResult struct {
	volume *api.StorageVolume
	etag   string
	err    error
}

type storageVolumeCreateArgs struct {
	pool   string
	volume api.StorageVolumesPost
}

func (f *fakeIncusAPI) GetServer() (*api.Server, string, error)               { return nil, "", nil }
func (f *fakeIncusAPI) GetInstance(string) (*api.Instance, string, error)     { return nil, "", nil }
func (f *fakeIncusAPI) GetInstances(api.InstanceType) ([]api.Instance, error) { return nil, nil }
func (f *fakeIncusAPI) CreateInstance(instance api.InstancesPost) (incusclient.Operation, error) {
	f.createInstanceArgs = append(f.createInstanceArgs, instance)
	return f.createInstanceOp, f.createInstanceErr
}
func (f *fakeIncusAPI) UpdateInstanceState(string, api.InstanceStatePut, string) (incusclient.Operation, error) {
	return f.updateInstanceStateOp, f.updateInstanceStateErr
}
func (f *fakeIncusAPI) DeleteInstance(string) (incusclient.Operation, error) {
	return fakeOperation{}, nil
}
func (f *fakeIncusAPI) ExecInstance(string, api.InstanceExecPost, *incusclient.InstanceExecArgs) (incusclient.Operation, error) {
	return f.execInstanceOp, f.execInstanceErr
}
func (f *fakeIncusAPI) GetInstanceFile(string, string) (io.ReadCloser, *incusclient.InstanceFileResponse, error) {
	return nil, nil, nil
}
func (f *fakeIncusAPI) CreateInstanceFile(string, string, incusclient.InstanceFileArgs) error {
	return f.createInstanceFileErr
}
func (f *fakeIncusAPI) DeleteInstanceFile(string, string) error { return f.deleteInstanceFileErr }
func (f *fakeIncusAPI) CreateInstanceFromImage(incusclient.ImageServer, api.Image, api.InstancesPost) (incusclient.RemoteOperation, error) {
	return f.createInstanceFromImageOp, f.createInstanceFromImageErr
}
func (f *fakeIncusAPI) CopyImage(incusclient.ImageServer, api.Image, *incusclient.ImageCopyArgs) (incusclient.RemoteOperation, error) {
	return fakeRemoteOperation{}, nil
}
func (f *fakeIncusAPI) GetImage(string) (*api.Image, string, error) {
	return f.getImageResult, "", f.getImageErr
}
func (f *fakeIncusAPI) GetImageAliases() ([]api.ImageAliasesEntry, error) { return nil, nil }
func (f *fakeIncusAPI) CreateImage(api.ImagesPost, *incusclient.ImageCreateArgs) (incusclient.Operation, error) {
	return fakeOperation{}, nil
}
func (f *fakeIncusAPI) CreateImageAlias(api.ImageAliasesPost) error { return nil }
func (f *fakeIncusAPI) DeleteImage(string) (incusclient.Operation, error) {
	return fakeOperation{}, nil
}
func (f *fakeIncusAPI) GetNetwork(string) (*api.Network, string, error)         { return nil, "", nil }
func (f *fakeIncusAPI) GetNetworks() ([]api.Network, error)                     { return nil, nil }
func (f *fakeIncusAPI) CreateNetwork(api.NetworksPost) error                    { return nil }
func (f *fakeIncusAPI) DeleteNetwork(string) error                              { return nil }
func (f *fakeIncusAPI) GetStoragePool(string) (*api.StoragePool, string, error) { return nil, "", nil }
func (f *fakeIncusAPI) GetStoragePools() ([]api.StoragePool, error)             { return nil, nil }
func (f *fakeIncusAPI) GetStoragePoolVolume(pool string, volType string, name string) (*api.StorageVolume, string, error) {
	if f.getStoragePoolVolumeCallCount < len(f.getStoragePoolVolumeResults) {
		result := f.getStoragePoolVolumeResults[f.getStoragePoolVolumeCallCount]
		f.getStoragePoolVolumeCallCount++
		return result.volume, result.etag, result.err
	}
	f.getStoragePoolVolumeCallCount++
	return nil, "", api.StatusErrorf(404, "not found")
}
func (f *fakeIncusAPI) CreateStoragePoolVolume(pool string, volume api.StorageVolumesPost) error {
	f.createStoragePoolVolumeCallCount++
	f.createStoragePoolVolumeArgs = append(f.createStoragePoolVolumeArgs, storageVolumeCreateArgs{pool: pool, volume: volume})
	return f.createStoragePoolVolumeErr
}
func (f *fakeIncusAPI) DeleteStoragePoolVolume(string, string, string) error     { return nil }
func (f *fakeIncusAPI) GetProfile(string) (*api.Profile, string, error)          { return nil, "", nil }
func (f *fakeIncusAPI) UseProject(string) incusclient.InstanceServer             { return nil }
func (f *fakeIncusAPI) UseTarget(string) incusclient.InstanceServer              { return nil }
func (f *fakeIncusAPI) GetInstanceLogfiles(string) ([]string, error)             { return nil, nil }
func (f *fakeIncusAPI) GetInstanceLogfile(string, string) (io.ReadCloser, error) { return nil, nil }
func (f *fakeIncusAPI) Disconnect()                                              {}

func TestEnsureVolumes_CreatesMissingVolumes(t *testing.T) {
	fake := &fakeIncusAPI{
		getStoragePoolVolumeResults: []storageVolumeResult{
			{err: api.StatusErrorf(404, "not found")},
			{err: api.StatusErrorf(404, "not found")},
		},
	}
	client := &Client{cli: fake, storagePool: "default", logger: boshlog.NewLogger(boshlog.LevelNone), logTag: "incusClient"}

	err := client.EnsureVolumes(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, fake.createStoragePoolVolumeCallCount)
}

func TestEnsureVolumes_ReturnsErrorOnCreateFailure(t *testing.T) {
	fake := &fakeIncusAPI{
		getStoragePoolVolumeResults: []storageVolumeResult{{err: api.StatusErrorf(404, "not found")}},
		createStoragePoolVolumeErr:  api.StatusErrorf(500, "create failed"),
	}
	client := &Client{cli: fake, storagePool: "default", logger: boshlog.NewLogger(boshlog.LevelNone), logTag: "incusClient"}

	err := client.EnsureVolumes(context.Background())
	require.Error(t, err)
}

func TestStartContainer_AddsVolumeDevices(t *testing.T) {
	fake := &fakeIncusAPI{
		getImageResult:        &api.Image{},
		createInstanceOp:      fakeOperation{},
		updateInstanceStateOp: fakeOperation{},
		execInstanceOp:        fakeOperation{},
		createInstanceFileErr: nil,
		deleteInstanceFileErr: nil,
		getStoragePoolVolumeResults: []storageVolumeResult{
			{err: api.StatusErrorf(404, "not found")},
			{err: api.StatusErrorf(404, "not found")},
		},
	}
	client := &Client{
		cli:         fake,
		storagePool: "default",
		networkName: "ibosh-net",
		imageName:   "fingerprint",
		logger:      boshlog.NewLogger(boshlog.LevelNone),
		logTag:      "incusClient",
	}

	confDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(confDir, "client.crt"), []byte("cert"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(confDir, "client.key"), []byte("key"), 0600))
	t.Setenv("INCUS_CONF", confDir)

	err := client.StartContainer(context.Background())
	require.NoError(t, err)
	require.Len(t, fake.createInstanceArgs, 1)
	require.Contains(t, fake.createInstanceArgs[0].Devices, "store")
	require.Contains(t, fake.createInstanceArgs[0].Devices, "data")
}
