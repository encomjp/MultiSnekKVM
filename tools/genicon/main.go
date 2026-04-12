//go:build ignore

// genicon generates build/appicon.png and build/windows/icon.ico
// matching the MultiSnek brand: dark zinc bg + sky-blue send-arrow outline.
// Run from repo root: go run ./tools/genicon/
package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
)

func main() {
	sizes := []int{256, 128, 64, 48, 32, 16}

	// Generate each size
	imgs := make([]image.Image, len(sizes))
	for i, sz := range sizes {
		imgs[i] = renderIcon(sz)
	}

	// Write appicon.png at 1024x1024
	big := renderIcon(1024)
	f, err := os.Create("build/appicon.png")
	must(err)
	must(png.Encode(f, big))
	f.Close()

	// Write icon.ico (multi-size)
	writeICO("build/windows/icon.ico", imgs, sizes)
}

// --- Rendering ---

func renderIcon(sz int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, sz, sz))

	radius := float64(sz) * 0.226 // ~58/256 corner radius ratio
	bg0 := color.NRGBA{24, 24, 27, 255}   // #18181b
	bg1 := color.NRGBA{9, 9, 11, 255}     // #09090b
	borderCol := color.NRGBA{255, 255, 255, 18}

	// Fill rounded rect background with radial gradient approximation
	cx, cy := float64(sz)/2, float64(sz)/2
	maxDist := math.Sqrt(cx*cx+cy*cy)
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			if !inRoundRect(x, y, sz, radius) {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
				continue
			}
			// Radial blend bg0→bg1
			dx, dy := float64(x)-cx, float64(y)-cy
			t := math.Sqrt(dx*dx+dy*dy) / maxDist
			col := lerpColor(bg0, bg1, t)
			img.SetNRGBA(x, y, col)
		}
	}

	// 1.5px border on rounded rect edge
	borderW := math.Max(1, float64(sz)*0.006)
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			inside := inRoundRect(x, y, sz, radius)
			insideInner := inRoundRect(x, y, sz, radius-borderW)
			if inside && !insideInner {
				img.SetNRGBA(x, y, borderCol)
			}
		}
	}

	// Draw the send-arrow icon
	drawArrow(img, sz)

	return img
}

// inRoundRect returns true if pixel (px,py) is inside a rounded rectangle.
func inRoundRect(px, py, sz int, r float64) bool {
	x, y := float64(px)+0.5, float64(py)+0.5
	s := float64(sz)
	// clamp to corner circles
	cx := clampF(x, r, s-r)
	cy := clampF(y, r, s-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// drawArrow draws the send-arrow path (M15 10l-4 4l6 6l4-16l-18 7l4 2l2 6l3-4)
// from a 24x24 viewBox, scaled to fill the icon nicely.
func drawArrow(img *image.NRGBA, sz int) {
	// Scale/offset: fit a ~18x18 bounding box of the path centered in the icon
	// Original path points (absolute): build the polyline vertices
	// The path segments ending vertices (from the SVG path):
	// Start: (15,10)
	// l-4,4  → (11,14)
	// l6,6   → (17,20)
	// l4,-16 → (21,4)
	// l-18,7 → (3,11)
	// l4,2   → (7,13)
	// l2,6   → (9,19)
	// l3,-4  → (12,15 = same as point after l-4,4, this closes the inner shape)
	//
	// Draw as two filled polygons:
	// Outer body: (15,10)→(11,14)→(17,20)→(21,4)→(3,11)→(7,13)→(9,19)→(12,15)→(15,10)

	s := float64(sz)
	// The path bounding box in viewbox coords: x: 3..21, y: 4..20 → 18x16
	// We want it to occupy ~60% of icon width, centered.
	vbW, vbH := 18.0, 16.0
	targetW := s * 0.62
	scaleX := targetW / vbW
	scaleY := targetW / vbH * (vbH / vbW)
	scale := math.Min(scaleX, scaleY)
	// Offset to center: bounding box origin in viewbox is (3,4)
	offX := (s-vbW*scale)/2 - 3*scale
	offY := (s-vbH*scale)/2 - 4*scale

	toImg := func(x, y float64) (float64, float64) {
		return x*scale + offX, y*scale + offY
	}

	// Sky-blue stroke color #38bdf8 with glow passes
	blue := color.NRGBA{56, 189, 248, 255}
	blueGlow := color.NRGBA{56, 189, 248, 60}

	// Path vertices (the full closed shape outline)
	pts := [][2]float64{
		{15, 10}, {11, 14}, {17, 20}, {21, 4}, {3, 11}, {7, 13}, {9, 19}, {12, 15}, {15, 10},
	}

	strokeWidth := math.Max(1.2, s*0.047)

	// Draw glow (wider, transparent)
	drawPolyline(img, pts, toImg, strokeWidth+float64(sz)*0.035, blueGlow)
	// Draw crisp stroke
	drawPolyline(img, pts, toImg, strokeWidth, blue)
}

func drawPolyline(img *image.NRGBA, pts [][2]float64, toImg func(float64, float64) (float64, float64), width float64, col color.NRGBA) {
	for i := 0; i < len(pts)-1; i++ {
		ax, ay := toImg(pts[i][0], pts[i][1])
		bx, by := toImg(pts[i+1][0], pts[i+1][1])
		drawLine(img, ax, ay, bx, by, width, col)
	}
}

func drawLine(img *image.NRGBA, x0, y0, x1, y1, width float64, col color.NRGBA) {
	bounds := img.Bounds()
	dx, dy := x1-x0, y1-y0
	length := math.Sqrt(dx*dx + dy*dy)
	if length == 0 {
		return
	}
	// Perpendicular unit vector
	nx, ny := -dy/length, dx/length
	half := width / 2.0
	steps := int(length*2) + 1
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		cx, cy := x0+dx*t, y0+dy*t
		// Rasterize a thick point across perpendicular
		r := int(math.Ceil(half)) + 1
		for py := -r; py <= r; py++ {
			for px := -r; px <= r; px++ {
				fx, fy := cx+float64(px), cy+float64(py)
				// Distance from segment center line (perpendicular distance)
				perpDist := (fx-cx)*nx + (fy-cy)*ny
				// Parallel distance from endpoints for round caps
				paraT := (fx-x0)*dx/length/length + (fy-y0)*dy/length/length
				var dist float64
				if paraT < 0 {
					dist = math.Sqrt((fx-x0)*(fx-x0) + (fy-y0)*(fy-y0))
				} else if paraT > 1 {
					dist = math.Sqrt((fx-x1)*(fx-x1) + (fy-y1)*(fy-y1))
				} else {
					dist = math.Abs(perpDist)
				}
				if dist > half+1 {
					continue
				}
				alpha := clampF(half+1-dist, 0, 1)
				ipx, ipy := int(fx), int(fy)
				if ipx < bounds.Min.X || ipx >= bounds.Max.X || ipy < bounds.Min.Y || ipy >= bounds.Max.Y {
					continue
				}
				blendPixel(img, ipx, ipy, col, alpha)
			}
		}
	}
}

func blendPixel(img *image.NRGBA, x, y int, src color.NRGBA, alpha float64) {
	a := float64(src.A) / 255.0 * alpha
	dst := img.NRGBAAt(x, y)
	oa := float64(dst.A) / 255.0
	na := a + oa*(1-a)
	if na < 0.001 {
		return
	}
	blend := func(s, d uint8) uint8 {
		return uint8((float64(s)*a + float64(d)*oa*(1-a)) / na)
	}
	img.SetNRGBA(x, y, color.NRGBA{
		R: blend(src.R, dst.R),
		G: blend(src.G, dst.G),
		B: blend(src.B, dst.B),
		A: uint8(na * 255),
	})
}

func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.NRGBA{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B), lerp(a.A, b.A)}
}

// --- ICO writer ---

func writeICO(path string, imgs []image.Image, sizes []int) {
	// Encode each image as PNG
	var pngs [][]byte
	for _, img := range imgs {
		var buf []byte
		pw := &byteWriter{buf: &buf}
		must(png.Encode(pw, img))
		pngs = append(pngs, buf)
	}

	f, err := os.Create(path)
	must(err)
	defer f.Close()

	n := len(imgs)
	// ICO header: 6 bytes
	write16(f, 0)    // reserved
	write16(f, 1)    // type: ICO
	write16(f, uint16(n)) // count

	// Directory entries: 16 bytes each
	dataOffset := uint32(6 + n*16)
	offsets := make([]uint32, n)
	for i := range pngs {
		offsets[i] = dataOffset
		dataOffset += uint32(len(pngs[i]))
	}

	for i, sz := range sizes {
		b := byte(sz)
		if sz == 256 {
			b = 0
		}
		f.Write([]byte{b, b, 0, 0}) // width, height, colorCount, reserved
		write16(f, 1)               // planes
		write16(f, 32)              // bitCount
		write32(f, uint32(len(pngs[i])))
		write32(f, offsets[i])
	}

	for _, p := range pngs {
		f.Write(p)
	}
}

type byteWriter struct{ buf *[]byte }

func (b *byteWriter) Write(p []byte) (int, error) {
	*b.buf = append(*b.buf, p...)
	return len(p), nil
}

func write16(f *os.File, v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	f.Write(buf[:])
}

func write32(f *os.File, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	f.Write(buf[:])
}

// draw needed to satisfy import
var _ = draw.Over

func must(err error) {
	if err != nil {
		panic(err)
	}
}
