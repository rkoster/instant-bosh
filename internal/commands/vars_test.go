package commands

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Variable Interpolation", func() {
	Describe("loadVarsFiles", func() {
		Context("when vars file does not exist", func() {
			It("returns an error", func() {
				_, err := loadVarsFiles([]string{"non-existent-file.yml"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read vars file"))
			})
		})
	})

	Describe("interpolateVars", func() {
		Context("with simple string variable", func() {
			It("replaces the placeholder", func() {
				content := []byte("network: ((network_name))")
				vars := map[string]interface{}{
					"network_name": "instant-bosh",
				}
				result, err := interpolateVars(content, vars)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal("network: instant-bosh"))
			})
		})

		Context("with integer variable", func() {
			It("replaces the placeholder with string representation", func() {
				content := []byte("workers: ((worker_count))")
				vars := map[string]interface{}{
					"worker_count": 5,
				}
				result, err := interpolateVars(content, vars)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal("workers: 5"))
			})
		})

		Context("with boolean variable", func() {
			It("replaces the placeholder with string representation", func() {
				content := []byte("enabled: ((feature_enabled))")
				vars := map[string]interface{}{
					"feature_enabled": true,
				}
				result, err := interpolateVars(content, vars)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal("enabled: true"))
			})
		})

		Context("with multiple variables", func() {
			It("replaces all placeholders", func() {
				content := []byte("name: ((name))\ncount: ((count))")
				vars := map[string]interface{}{
					"name":  "test",
					"count": 3,
				}
				result, err := interpolateVars(content, vars)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal("name: test\ncount: 3"))
			})
		})

		Context("with no matching variables", func() {
			It("leaves the content unchanged", func() {
				content := []byte("name: test")
				vars := map[string]interface{}{
					"unused": "value",
				}
				result, err := interpolateVars(content, vars)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal("name: test"))
			})
		})

		Context("with missing variable", func() {
			It("leaves the placeholder unchanged", func() {
				content := []byte("name: ((missing_var))")
				vars := map[string]interface{}{}
				result, err := interpolateVars(content, vars)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(result)).To(Equal("name: ((missing_var))"))
			})
		})
	})
})
