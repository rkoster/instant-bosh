package uploadedrelease_test

import (
	"errors"
	"testing"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	semver "github.com/cppforlife/go-semi-semantic/version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/instant-bosh/internal/uploadedrelease"
)

func TestUploadedRelease(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UploadedRelease Suite")
}

// fakeRelease implements boshdir.Release for testing
type fakeRelease struct {
	name    string
	version semver.Version
}

func (r *fakeRelease) Name() string                          { return r.name }
func (r *fakeRelease) Version() semver.Version               { return r.version }
func (r *fakeRelease) Exists() (bool, error)                 { return true, nil }
func (r *fakeRelease) VersionMark(mark string) string        { return "" }
func (r *fakeRelease) CommitHashWithMark(mark string) string { return "" }
func (r *fakeRelease) Jobs() ([]boshdir.Job, error)          { return nil, nil }
func (r *fakeRelease) Packages() ([]boshdir.Package, error)  { return nil, nil }
func (r *fakeRelease) Delete(force bool) error               { return nil }

// fakeReleaseChecker implements uploadedrelease.ReleaseChecker for testing
type fakeReleaseChecker struct {
	releases []boshdir.Release
	err      error
}

func (f *fakeReleaseChecker) Releases() ([]boshdir.Release, error) {
	return f.releases, f.err
}

func newFakeRelease(name, version string) *fakeRelease {
	v, _ := semver.NewVersionFromString(version)
	return &fakeRelease{name: name, version: v}
}

var _ = Describe("Filter", func() {
	var (
		checker *fakeReleaseChecker
	)

	BeforeEach(func() {
		checker = &fakeReleaseChecker{}
	})

	Context("when releases exist on the director", func() {
		BeforeEach(func() {
			checker.releases = []boshdir.Release{
				newFakeRelease("bpm", "1.2.0"),
				newFakeRelease("diego", "2.50.0"),
			}
		})

		It("removes url and sha1 from existing releases", func() {
			manifest := `---
name: cf
releases:
- name: bpm
  version: "1.2.0"
  url: https://bosh.io/d/github.com/cloudfoundry/bpm-release?v=1.2.0
  sha1: abc123
- name: diego
  version: "2.50.0"
  url: https://bosh.io/d/github.com/cloudfoundry/diego-release?v=2.50.0
  sha1: def456
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("name: bpm"))
			Expect(resultStr).To(MatchRegexp(`version: "?1\.2\.0"?`))
			Expect(resultStr).NotTo(ContainSubstring("url: https://bosh.io/d/github.com/cloudfoundry/bpm-release"))
			Expect(resultStr).NotTo(ContainSubstring("sha1: abc123"))

			Expect(resultStr).To(ContainSubstring("name: diego"))
			Expect(resultStr).To(MatchRegexp(`version: "?2\.50\.0"?`))
			Expect(resultStr).NotTo(ContainSubstring("url: https://bosh.io/d/github.com/cloudfoundry/diego-release"))
			Expect(resultStr).NotTo(ContainSubstring("sha1: def456"))
		})

		It("preserves stemcell block in releases", func() {
			manifest := `---
name: cf
releases:
- name: bpm
  version: "1.2.0"
  url: https://example.com/bpm.tgz
  sha1: abc123
  stemcell:
    os: ubuntu-jammy
    version: "1.100"
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("stemcell:"))
			Expect(resultStr).To(ContainSubstring("os: ubuntu-jammy"))
			Expect(resultStr).To(MatchRegexp(`version: "?1\.100"?`))
			Expect(resultStr).NotTo(ContainSubstring("url:"))
			Expect(resultStr).NotTo(ContainSubstring("sha1:"))
		})
	})

	Context("when some releases do not exist on the director", func() {
		BeforeEach(func() {
			checker.releases = []boshdir.Release{
				newFakeRelease("bpm", "1.2.0"),
			}
		})

		It("only removes url and sha1 from existing releases", func() {
			manifest := `---
name: cf
releases:
- name: bpm
  version: "1.2.0"
  url: https://example.com/bpm.tgz
  sha1: abc123
- name: capi
  version: "1.100.0"
  url: https://example.com/capi.tgz
  sha1: xyz789
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())

			resultStr := string(result)
			// bpm should have url/sha1 removed
			Expect(resultStr).To(ContainSubstring("name: bpm"))
			Expect(resultStr).NotTo(ContainSubstring("url: https://example.com/bpm.tgz"))
			Expect(resultStr).NotTo(ContainSubstring("sha1: abc123"))

			// capi should keep url/sha1
			Expect(resultStr).To(ContainSubstring("name: capi"))
			Expect(resultStr).To(ContainSubstring("url: https://example.com/capi.tgz"))
			Expect(resultStr).To(ContainSubstring("sha1: xyz789"))
		})
	})

	Context("when no releases exist on the director", func() {
		BeforeEach(func() {
			checker.releases = []boshdir.Release{}
		})

		It("keeps all releases unchanged", func() {
			manifest := `---
name: cf
releases:
- name: bpm
  version: "1.2.0"
  url: https://example.com/bpm.tgz
  sha1: abc123
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("url: https://example.com/bpm.tgz"))
			Expect(resultStr).To(ContainSubstring("sha1: abc123"))
		})
	})

	Context("when the director returns an error", func() {
		BeforeEach(func() {
			checker.err = errors.New("connection refused")
		})

		It("returns the original manifest unchanged", func() {
			manifest := `---
name: cf
releases:
- name: bpm
  version: "1.2.0"
  url: https://example.com/bpm.tgz
  sha1: abc123
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal(manifest))
		})
	})

	Context("when manifest has no releases section", func() {
		BeforeEach(func() {
			checker.releases = []boshdir.Release{
				newFakeRelease("bpm", "1.2.0"),
			}
		})

		It("returns the manifest unchanged", func() {
			manifest := `---
name: cf
stemcells:
- alias: default
  os: ubuntu-jammy
  version: latest
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("name: cf"))
			Expect(resultStr).To(ContainSubstring("stemcells:"))
		})
	})

	Context("when release version is different", func() {
		BeforeEach(func() {
			checker.releases = []boshdir.Release{
				newFakeRelease("bpm", "1.1.0"), // Different version
			}
		})

		It("keeps url and sha1 for releases with non-matching versions", func() {
			manifest := `---
name: cf
releases:
- name: bpm
  version: "1.2.0"
  url: https://example.com/bpm.tgz
  sha1: abc123
`
			result, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).NotTo(HaveOccurred())

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("url: https://example.com/bpm.tgz"))
			Expect(resultStr).To(ContainSubstring("sha1: abc123"))
		})
	})

	Context("when manifest is invalid YAML", func() {
		It("returns an error", func() {
			manifest := `not: valid: yaml: [[[`
			_, err := uploadedrelease.Filter([]byte(manifest), checker)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse manifest"))
		})
	})
})
