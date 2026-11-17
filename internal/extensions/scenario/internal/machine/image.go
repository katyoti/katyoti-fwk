// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"fmt"
	"image"
	_ "image/png" // a stage's screenshots are PNG
	"os"
)

// The grid an image is read in: imageGrid x imageGrid tiles. A tile holds
// content when no one colour covers flatShare of it; an image shows something
// when at least contentShare of its tiles hold content. A screen that printed
// blank — all white, or a window's title bar over an empty body — has almost
// none.
const (
	imageGrid    = 16
	flatShare    = 0.98
	contentShare = 0.20
)

// CheckImage fails unless path is an image that shows something: it decodes,
// and enough of it is not one flat colour. A screenshot that captured nothing
// is a failure, whatever the command that took it printed.
func CheckImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("image %s: %w", path, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("image %s: %w", path, err)
	}
	b := img.Bounds()
	if b.Dx() < imageGrid || b.Dy() < imageGrid {
		return fmt.Errorf("image %s is %dx%d: too small to show anything", path, b.Dx(), b.Dy())
	}
	share := ContentShare(img)
	if share < contentShare {
		return fmt.Errorf("image %s is blank: %.0f%% of it holds content (at least %.0f%% expected)", path, share*100, contentShare*100)
	}
	return nil
}

// ContentShare is the share of an image's tiles (imageGrid x imageGrid) that
// hold content: no one colour covers flatShare of the tile.
func ContentShare(img image.Image) float64 {
	b := img.Bounds()
	content := 0
	for ty := 0; ty < imageGrid; ty++ {
		for tx := 0; tx < imageGrid; tx++ {
			x0, x1 := b.Min.X+b.Dx()*tx/imageGrid, b.Min.X+b.Dx()*(tx+1)/imageGrid
			y0, y1 := b.Min.Y+b.Dy()*ty/imageGrid, b.Min.Y+b.Dy()*(ty+1)/imageGrid
			counts := map[[4]uint32]int{}
			top := 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bl, a := img.At(x, y).RGBA()
					k := [4]uint32{r >> 8, g >> 8, bl >> 8, a >> 8}
					counts[k]++
					if counts[k] > top {
						top = counts[k]
					}
				}
			}
			if n := (x1 - x0) * (y1 - y0); n > 0 && float64(top) < flatShare*float64(n) {
				content++
			}
		}
	}
	return float64(content) / float64(imageGrid*imageGrid)
}
