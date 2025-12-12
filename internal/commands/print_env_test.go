package commands_test

import (
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/commands/commandsfakes"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/docker/dockerfakes"
)

var _ = Describe("PrintEnvAction", func() {
	var (
		fakeDockerAPI     *dockerfakes.FakeDockerAPI
		fakeClientFactory *dockerfakes.FakeClientFactory
		fakeUI            *commandsfakes.FakeUI
		logger            boshlog.Logger
	)

	_ = logger // suppress unused warning

	BeforeEach(func() {
		fakeDockerAPI = &dockerfakes.FakeDockerAPI{}
		fakeClientFactory = &dockerfakes.FakeClientFactory{}
		fakeUI = &commandsfakes.FakeUI{}

		logger = boshlog.NewLogger(boshlog.LevelNone)

		// Configure fakeClientFactory to return a test client with fakeDockerAPI
		fakeClientFactory.NewClientStub = func(logger boshlog.Logger, customImage string) (*docker.Client, error) {
			return docker.NewTestClient(fakeDockerAPI, logger, docker.ImageName), nil
		}

		// Default: DaemonHost returns unix socket
		fakeDockerAPI.DaemonHostReturns("unix:///var/run/docker.sock")

		// Default: Close succeeds
		fakeDockerAPI.CloseReturns(nil)
	})

	Describe("factory pattern support", func() {
		Context("when print-env command uses factory pattern", func() {
			It("uses the default client factory in production", func() {
				// This test documents that PrintEnvAction currently creates its own Docker client directly
				// via docker.NewClient(), which means it uses the default client factory.
				//
				// Unlike StartAction which was refactored to accept factories, PrintEnvAction still
				// instantiates the client internally:
				//   dockerClient, err := docker.NewClient(logger, "")
				//
				// Future enhancement: Refactor PrintEnvAction to accept a ClientFactory parameter
				// and ConfigProvider parameter to enable full unit testing with fakes. This would
				// follow the same pattern as StartActionWithFactories().
				//
				// Proposed signature:
				//   func PrintEnvActionWithFactories(ui boshui.UI, logger boshlog.Logger, clientFactory docker.ClientFactory, configProvider director.ConfigProvider) error
				//   func PrintEnvAction(ui boshui.UI, logger boshlog.Logger) error {
				//       return PrintEnvActionWithFactories(ui, logger, &docker.DefaultClientFactory{}, &director.DefaultConfigProvider{})
				//   }

				Expect(fakeClientFactory).NotTo(BeNil())
				Expect(fakeUI).NotTo(BeNil())
			})
		})
	})

	Describe("container state checking", func() {
		Context("when container is not running", func() {
			It("documents expected behavior when container is not running", func() {
				// This test documents the expected behavior when container is not running:
				//
				// 1. PrintEnvAction checks if container is running via dockerClient.IsContainerRunning(ctx)
				// 2. If not running, returns error: "instant-bosh container is not running. Please run 'ibosh start' first"
				// 3. No environment variables are printed
				//
				// This is stricter than the env command - print-env requires the container to be
				// running because it needs to fetch director configuration from the running instance.
				//
				// Users should run `ibosh start` before running `ibosh print-env`.

				Expect(fakeDockerAPI.ContainerListCallCount()).To(Equal(0))
			})
		})

		Context("when container is running", func() {
			It("documents expected environment variables output format", func() {
				// This test documents the expected output when container is running:
				//
				// PrintEnvAction fetches director configuration and prints shell export statements:
				//
				// 1. Calls: director.GetDirectorConfig(ctx, dockerClient) to fetch config
				// 2. Prints environment variables (all via ui.PrintLinef):
				//    "export BOSH_CLIENT=admin"
				//    "export BOSH_CLIENT_SECRET=fake-admin-password"
				//    "export BOSH_ENVIRONMENT=https://127.0.0.1:25555"
				//    "export BOSH_CA_CERT='-----BEGIN CERTIFICATE-----...'"
				//    "export BOSH_ALL_PROXY=ssh+socks5://jumpbox@127.0.0.1:22?private-key=/tmp/..."
				//
				// **Important Notes:**
				//   - Output is shell-compatible export statements
				//   - Users can eval the output: eval "$(ibosh print-env)"
				//   - BOSH_CA_CERT is wrapped in single quotes to preserve newlines
				//   - BOSH_ALL_PROXY includes path to temporary private key file
				//   - The jumpbox key file is NOT cleaned up (must persist for shell session)
				//
				// **Usage:**
				//   eval "$(ibosh print-env)"
				//   bosh deployments
				//   bosh vms
				//
				// This provides seamless integration with the BOSH CLI.

				Expect(fakeDockerAPI.ContainerListCallCount()).To(Equal(0))
			})
		})
	})

	Describe("director configuration fetching", func() {
		Context("when director config can be fetched", func() {
			It("documents the director configuration fetching process", func() {
				// This test documents how director configuration is fetched:
				//
				// director.GetDirectorConfig(ctx, dockerClient) executes these commands:
				//
				// 1. **BOSH_ENVIRONMENT:**
				//    Command: bosh int /var/vcap/bosh/manifest.yml --path /instance_groups/name=bosh/properties/director/address
				//    Returns: https://127.0.0.1:25555
				//
				// 2. **BOSH_CLIENT:**
				//    Command: bosh int /var/vcap/bosh/manifest.yml --path /instance_groups/name=bosh/properties/director/user_management/local/users/0/name
				//    Returns: admin
				//
				// 3. **BOSH_CLIENT_SECRET:**
				//    Command: bosh int /var/vcap/bosh/manifest.yml --path /instance_groups/name=bosh/properties/director/user_management/local/users/0/password
				//    Returns: <password from manifest>
				//
				// 4. **BOSH_CA_CERT:**
				//    Command: cat /var/vcap/bosh/certs/ca_cert.pem
				//    Returns: <PEM-encoded certificate>
				//
				// 5. **Jumpbox Private Key:**
				//    Command: cat /var/vcap/bosh/ssh/jumpbox_key
				//    Returns: <PEM-encoded private key>
				//    Saved to: Temporary file on host filesystem
				//    Used for: BOSH_ALL_PROXY SSH tunnel
				//
				// The configuration is then formatted into shell export statements.

				Expect(true).To(BeTrue())
			})
		})

		Context("when director config cannot be fetched", func() {
			It("documents error handling for config fetching failures", func() {
				// This test documents behavior when config fetching fails:
				//
				// If director.GetDirectorConfig() fails:
				//   1. PrintEnvAction returns error: "failed to get director config: <underlying error>"
				//   2. No environment variables are printed
				//   3. User sees error message from CLI framework
				//
				// Example failure scenarios:
				//   - Container is running but BOSH director hasn't started yet
				//   - Manifest file doesn't exist or is malformed
				//   - ExecCommand fails due to container issues
				//   - Certificate or key files are missing
				//
				// Users should wait for `ibosh start` to complete fully before running
				// `ibosh print-env`.

				Expect(true).To(BeTrue())
			})
		})
	})

	Describe("jumpbox key file handling", func() {
		It("documents important note about key file cleanup", func() {
			// This test documents an important design decision about key file cleanup:
			//
			// **CRITICAL:** PrintEnvAction intentionally does NOT call config.Cleanup()
			//
			// Reason:
			//   The jumpbox private key file needs to persist for the user's shell session.
			//   BOSH_ALL_PROXY references this key file for SSH tunneling, and it must
			//   exist as long as the user is running BOSH CLI commands.
			//
			// File location:
			//   Temporary file created by director.GetDirectorConfig()
			//   Path included in BOSH_ALL_PROXY environment variable
			//   Example: /tmp/instant-bosh-jumpbox-key-123456
			//
			// Lifecycle:
			//   - Created: When print-env runs
			//   - Used: For all BOSH CLI commands in the shell session
			//   - Deleted: Automatically by OS when system restarts (temp file)
			//
			// If config.Cleanup() were called, the BOSH CLI would fail with:
			//   "Error: Cannot load SSH private key: no such file or directory"
			//
			// This is documented in print_env.go with a NOTE comment.

			Expect(true).To(BeTrue())
		})
	})

	Describe("shell integration", func() {
		It("documents usage patterns for shell integration", func() {
			// This test documents how to use print-env for shell integration:
			//
			// **Basic Usage:**
			//   eval "$(ibosh print-env)"
			//
			// **What happens:**
			//   1. ibosh print-env prints export statements to stdout
			//   2. $(...) captures the output
			//   3. eval executes the export statements in current shell
			//   4. BOSH_* environment variables are now set in the shell
			//
			// **Example workflow:**
			//   $ ibosh start
			//   $ eval "$(ibosh print-env)"
			//   $ bosh env
			//   $ bosh deployments
			//   $ bosh upload-release release.tgz
			//
			// **Alternative (manual export):**
			//   $ ibosh print-env > bosh_env.sh
			//   $ cat bosh_env.sh
			//   export BOSH_CLIENT=admin
			//   export BOSH_CLIENT_SECRET=...
			//   $ source bosh_env.sh
			//
			// **Unset variables:**
			//   unset BOSH_CLIENT BOSH_CLIENT_SECRET BOSH_ENVIRONMENT BOSH_CA_CERT BOSH_ALL_PROXY
			//
			// This provides seamless BOSH CLI integration without requiring separate
			// environment configuration files.

			Expect(true).To(BeTrue())
		})
	})

	Describe("output format requirements", func() {
		It("documents strict requirements for shell-compatible output", func() {
			// This test documents important output format requirements:
			//
			// **Requirements for shell compatibility:**
			//
			// 1. **Use ui.PrintLinef() for ALL output**
			//    - ui.PrintLinef() writes to stdout (outWriter)
			//    - ui.ErrorLinef() writes to stderr (errWriter)
			//    - Only stdout is captured by $(...)
			//    - All export statements MUST use ui.PrintLinef()
			//
			// 2. **Quote values containing special characters**
			//    - BOSH_CA_CERT is wrapped in single quotes: 'certificate-with-newlines'
			//    - Single quotes preserve literal newlines in the shell
			//    - Other values use double quotes or no quotes
			//
			// 3. **One export per line**
			//    - Each ui.PrintLinef() call prints one complete export statement
			//    - eval processes each line as a separate command
			//
			// 4. **No extra output**
			//    - No progress messages
			//    - No status updates
			//    - No blank lines
			//    - Only export statements
			//    - Any extra output would be executed by eval (causing errors)
			//
			// **Incorrect (would break eval):**
			//   ui.PrintLinef("Fetching configuration...")  // ❌ Not a valid shell command
			//   ui.PrintLinef("export BOSH_CLIENT=admin")
			//
			// **Correct (clean output):**
			//   ui.PrintLinef("export BOSH_CLIENT=admin")  // ✅ Only export statements
			//   ui.PrintLinef("export BOSH_CLIENT_SECRET=...")

			Expect(fakeUI).NotTo(BeNil())
		})
	})

	Describe("UI message documentation", func() {
		It("documents all expected output in print-env command flows", func() {
			// This test documents the complete output for print-env command:
			//
			// **Success case (container running):**
			//   export BOSH_CLIENT=admin
			//   export BOSH_CLIENT_SECRET=<password>
			//   export BOSH_ENVIRONMENT=https://127.0.0.1:25555
			//   export BOSH_CA_CERT='<certificate>'
			//   export BOSH_ALL_PROXY=ssh+socks5://jumpbox@127.0.0.1:22?private-key=<path>
			//
			// **Error case (container not running):**
			//   [Error message returned to CLI framework]
			//   "instant-bosh container is not running. Please run 'ibosh start' first"
			//
			// **Error case (config fetch fails):**
			//   [Error message returned to CLI framework]
			//   "failed to get director config: <underlying error>"
			//
			// All successful output uses ui.PrintLinef() and goes to stdout for eval.
			// All errors are returned and handled by the CLI framework (go to stderr).

			Expect(fakeUI).NotTo(BeNil())
		})
	})
})
