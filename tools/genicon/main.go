// Command genicon generates assets/icon.ico and assets/icon.png (standard
// library only).
//
// The icon is a blue rounded square, the same gradient as Simple RTSP
// Viewer's, with a white play triangle between two yellow trim brackets:
// a clip and the handles that trim it. Each size is rendered directly with
// 4x4 supersampling, so the small sizes stay sharp.
//
// Usage, from the repository root:
//
//	go run ./tools/genicon
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var icoSizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}

const samples = 4

var (
	top    = [3]float64{0x2B, 0x88, 0xD8}
	bottom = [3]float64{0x00, 0x5A, 0xA6}
	white  = [3]float64{0xFF, 0xFF, 0xFF}
	yellow = [3]float64{0xFF, 0xC8, 0x3D}
)

type point struct{ x, y float64 }

// Shapes in unit coordinates (0..1).
var (
	play = []point{{0.42, 0.33}, {0.42, 0.67}, {0.67, 0.50}}

	// Each bracket is a vertical bar with short arms at its ends.
	leftBracket = [][4]float64{
		{0.17, 0.25, 0.26, 0.75}, // bar: x0, y0, x1, y1
		{0.17, 0.25, 0.35, 0.34}, // top arm
		{0.17, 0.66, 0.35, 0.75}, // bottom arm
	}
	rightBracket = [][4]float64{
		{0.74, 0.25, 0.83, 0.75},
		{0.65, 0.25, 0.83, 0.34},
		{0.65, 0.66, 0.83, 0.75},
	}
)

func inRoundedRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := min(max(x, x0+r), x1-r)
	cy := min(max(y, y0+r), y1-r)
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

func inPolygon(x, y float64, pts []point) bool {
	inside := false
	j := len(pts) - 1
	for i, p := range pts {
		q := pts[j]
		if (p.y > y) != (q.y > y) && x < (q.x-p.x)*(y-p.y)/(q.y-p.y)+p.x {
			inside = !inside
		}
		j = i
	}
	return inside
}

func inBracket(x, y float64, parts [][4]float64) bool {
	for _, r := range parts {
		if inRoundedRect(x, y, r[0], r[1], r[2], r[3], 0.015) {
			return true
		}
	}
	return false
}

// shade returns the color of a point in unit coordinates, or false outside
// the icon.
func shade(x, y float64) ([3]float64, bool) {
	if !inRoundedRect(x, y, 0.03, 0.03, 0.97, 0.97, 0.22) {
		return [3]float64{}, false
	}
	switch {
	case inBracket(x, y, leftBracket) || inBracket(x, y, rightBracket):
		return yellow, true
	case inPolygon(x, y, play):
		return white, true
	}
	t := (y - 0.03) / 0.94
	var c [3]float64
	for i := range c {
		c[i] = top[i] + (bottom[i]-top[i])*t
	}
	return c, true
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := range size {
		for px := range size {
			var sum [3]float64
			hits := 0
			for sy := range samples {
				for sx := range samples {
					x := (float64(px) + (float64(sx)+0.5)/samples) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/samples) / float64(size)
					if c, ok := shade(x, y); ok {
						for i := range sum {
							sum[i] += c[i]
						}
						hits++
					}
				}
			}
			if hits == 0 {
				continue
			}
			n := float64(hits)
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(sum[0]/n + 0.5),
				G: uint8(sum[1]/n + 0.5),
				B: uint8(sum[2]/n + 0.5),
				A: uint8(255*n/samples/samples + 0.5),
			})
		}
	}
	return img
}

// ico builds an .ico file with PNG-compressed images.
func ico(sizes []int, pngs [][]byte) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, uint16(len(pngs))})
	offset := 6 + 16*len(pngs)
	for i, data := range pngs {
		dim := uint8(sizes[i])
		if sizes[i] >= 256 {
			dim = 0 // 0 means 256
		}
		b.Write([]byte{dim, dim, 0, 0})
		binary.Write(&b, le, [2]uint16{1, 32})
		binary.Write(&b, le, [2]uint32{uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range pngs {
		b.Write(data)
	}
	return b.Bytes()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "genicon:", err)
		os.Exit(1)
	}
}

func run() error {
	pngs := make([][]byte, len(icoSizes))
	for i, size := range icoSizes {
		var b bytes.Buffer
		if err := png.Encode(&b, render(size)); err != nil {
			return err
		}
		pngs[i] = b.Bytes()
	}
	dir := "assets"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.ico"), ico(icoSizes, pngs), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.png"), pngs[len(pngs)-1], 0o644); err != nil {
		return err
	}
	fmt.Println("wrote assets/icon.ico and assets/icon.png")
	return nil
}
