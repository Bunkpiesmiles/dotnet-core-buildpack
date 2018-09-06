package dotnetaspnetcore_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDotnetaspnetcore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dotnetaspnetcore Suite")
}
