package commands

import (
	"github.com/cloudfoundry/bosh-cli/v7/ui/table"
)

// UI is a wrapper interface for boshui.UI that allows us to generate fakes for testing.
// This interface defines the subset of boshui.UI methods that we use in the commands package.
//
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . UI
type UI interface {
	PrintLinef(pattern string, args ...interface{})
	ErrorLinef(pattern string, args ...interface{})
	BeginLinef(pattern string, args ...interface{})
	EndLinef(pattern string, args ...interface{})
	PrintBlock(block []byte)
	PrintErrorBlock(block string)
	PrintTable(table table.Table)
	AskForConfirmation() error
	AskForConfirmationWithLabel(label string) error
	AskForChoice(label string, options []string) (int, error)
	IsInteractive() bool
	Flush()
}
