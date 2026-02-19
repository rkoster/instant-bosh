package commands

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"

	"github.com/qeesung/image2ascii/convert"
	"golang.org/x/term"
)

//go:embed assets/logo.png
var logoData []byte

// PrintLogo displays the instant-bosh logo in ASCII art to stdout
func PrintLogo() error {
	return PrintLogoTo(os.Stdout)
}

// PrintLogoTo displays the instant-bosh logo in ASCII art to the given writer
func PrintLogoTo(w io.Writer) error {
	// Decode the embedded image
	img, _, err := image.Decode(bytes.NewReader(logoData))
	if err != nil {
		return fmt.Errorf("failed to decode logo image: %w", err)
	}

	// Get terminal dimensions
	termWidth, termHeight, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || termWidth <= 0 || termHeight <= 0 {
		// Fallback to defaults if terminal size detection fails
		termWidth = 120
		termHeight = 30
	}

	// Calculate dimensions based on available height
	// Use 80% of terminal height
	maxHeight := int(float64(termHeight) * 0.8)
	if maxHeight < 10 {
		maxHeight = 10
	}

	// Calculate width based on image aspect ratio and terminal character aspect ratio
	// Terminal characters are roughly 2:1 (height:width), so we adjust accordingly
	bounds := img.Bounds()
	imageAspectRatio := float64(bounds.Dx()) / float64(bounds.Dy())
	width := int(float64(maxHeight) * imageAspectRatio * 2.0) // 2.0 factor for terminal character aspect ratio

	// Cap at 130 characters width
	if width > 130 {
		width = 130
		// Recalculate height based on capped width
		maxHeight = int(float64(width) / imageAspectRatio * 0.5)
	}

	// Also ensure we don't exceed terminal width
	if width > termWidth {
		width = termWidth
		// Recalculate height based on terminal width
		maxHeight = int(float64(width) / imageAspectRatio * 0.5)
	}

	height := maxHeight

	// Create converter options
	convertOptions := convert.DefaultOptions
	convertOptions.FixedWidth = width
	convertOptions.FixedHeight = height
	convertOptions.Colored = true

	// Create converter
	converter := convert.NewImageConverter()

	// Convert image to ASCII
	asciiArt := converter.Image2ASCIIString(img, &convertOptions)

	// Print to writer
	fmt.Fprintln(w, asciiArt)
	return nil
}
