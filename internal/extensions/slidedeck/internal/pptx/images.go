// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"path"
	"strings"

	"golang.org/x/image/draw"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// maxAsset is the size a theme file may have: the host fetches a theme's
// files from the sidecar within the control band.
const maxAsset = contribapi.BandControl

// maxPixels bounds the pictures decoded: a tiny file may declare a huge
// image, which decoding would allocate whole.
const maxPixels = 40_000_000

// decode reads a PNG or JPEG picture; other formats (EMF, WMF, SVG) are not
// read, nor a picture above maxPixels.
func decode(b []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("a %d×%d picture is too large to read", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// average is a picture's mean colour, as #rrggbb, sampled on a grid.
func average(img image.Image) string {
	r := img.Bounds()
	var sr, sg, sb, n float64
	step := max(1, max(r.Dx(), r.Dy())/64)
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 128 {
				continue
			}
			sr, sg, sb, n = sr+float64(c.R), sg+float64(c.G), sb+float64(c.B), n+1
		}
	}
	if n == 0 {
		return "#FFFFFF"
	}
	return fmt.Sprintf("#%02X%02X%02X", int(sr/n+0.5), int(sg/n+0.5), int(sb/n+0.5))
}

// background re-encodes a background picture as a JPEG at most the slide's
// width, lowering the quality until it fits the control band.
func backgroundJPEG(img image.Image, width int) ([]byte, error) {
	img = scaleTo(img, width)
	for q := 85; ; q -= 10 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if buf.Len() <= maxAsset || q <= 45 {
			return buf.Bytes(), nil
		}
	}
}

// fitAsset keeps a picture as it is when it fits the control band, else
// halves it (PNG keeps its alpha) until it does.
func fitAsset(name string, b []byte) ([]byte, error) {
	if len(b) <= maxAsset {
		return b, nil
	}
	img, err := decode(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %d bytes, above the control band, and not a PNG or JPEG to shrink", name, len(b))
	}
	w := img.Bounds().Dx()
	for w > 16 {
		w /= 2
		var buf bytes.Buffer
		small := scaleTo(img, w)
		if strings.EqualFold(path.Ext(name), ".png") {
			err = png.Encode(&buf, small)
		} else {
			err = jpeg.Encode(&buf, small, &jpeg.Options{Quality: 85})
		}
		if err != nil {
			return nil, err
		}
		if buf.Len() <= maxAsset {
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("%s: cannot be brought under the control band", name)
}

// scaleTo scales a picture down to width (never up).
func scaleTo(img image.Image, width int) image.Image {
	r := img.Bounds()
	if r.Dx() <= width {
		return img
	}
	h := int(math.Round(float64(r.Dy()) * float64(width) / float64(r.Dx())))
	dst := image.NewNRGBA(image.Rect(0, 0, width, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, r, draw.Over, nil)
	return dst
}

// colourNames name a background by its nearest colour: what a cover variant
// is called (bordeaux, sand, cream…). A guess the report lists.
var colourNames = []struct {
	name    string
	r, g, b float64
}{
	{"white", 255, 255, 255}, {"cream", 240, 232, 220}, {"sand", 200, 180, 155},
	{"grey", 128, 128, 128}, {"black", 20, 20, 20}, {"bordeaux", 120, 10, 40},
	{"red", 220, 35, 35}, {"orange", 240, 130, 30}, {"yellow", 240, 210, 50},
	{"green", 40, 140, 70}, {"teal", 0, 128, 128}, {"blue", 30, 90, 200},
	{"navy", 20, 35, 90}, {"purple", 110, 40, 130}, {"brown", 110, 70, 40},
}

// colourName is the name of the colour nearest to c (#rrggbb).
func colourName(c string) string {
	r, g, b, ok := rgb(c)
	if !ok {
		return "plain"
	}
	best, dist := "plain", math.MaxFloat64
	for _, n := range colourNames {
		d := (r-n.r)*(r-n.r) + (g-n.g)*(g-n.g) + (b-n.b)*(b-n.b)
		if d < dist {
			best, dist = n.name, d
		}
	}
	return best
}

// rgb reads #rrggbb.
func rgb(c string) (r, g, b float64, ok bool) {
	var ri, gi, bi int
	if _, err := fmt.Sscanf(strings.TrimPrefix(c, "#"), "%02x%02x%02x", &ri, &gi, &bi); err != nil {
		return 0, 0, 0, false
	}
	return float64(ri), float64(gi), float64(bi), true
}

// luminance is a colour's lightness, 0 (black) to 1 (white).
func luminance(c string) float64 {
	r, g, b, ok := rgb(c)
	if !ok {
		return 0.5
	}
	return (0.2126*r + 0.7152*g + 0.0722*b) / 255
}
