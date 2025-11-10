package internal_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstantBosh(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InstantBosh Suite")
}
