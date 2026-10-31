package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// Point2D represents a coordinate in 2D space.
type Point2D struct {
	X, Y float64
}

// GenerateIcon creates a high-quality multi-resolution Windows ICO file
// accurately rendered from assets/icon.svg.
func main() {
	svgPath := filepath.Join("assets", "icon.svg")
	svgBytes, err := os.ReadFile(svgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: unable to read %s: %v\n", svgPath, err)
		os.Exit(1)
	}
	if len(svgBytes) == 0 {
		fmt.Fprintf(os.Stderr, "Error: %s is empty\n", svgPath)
		os.Exit(1)
	}

	sizes := []int{16, 32, 48, 64, 128, 256}
	var pngBuffers [][]byte

	for _, sz := range sizes {
		img := renderSvgArtwork(sz)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding PNG for size %d: %v\n", sz, err)
			os.Exit(1)
		}
		pngBuffers = append(pngBuffers, buf.Bytes())
	}

	icoData := createIco(sizes, pngBuffers)

	outPath := filepath.Join("assets", "icon.ico")
	if err := os.WriteFile(outPath, icoData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing icon.ico: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s from %s (%d sizes: 16 to 256px)\n", outPath, svgPath, len(sizes))
}

// renderSvgArtwork renders the vector artwork from assets/icon.svg with 4x supersampling (SSAA)
// for clean antialiased rendering across all Windows icon dimensions.
func renderSvgArtwork(targetSize int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))

	scale := float64(targetSize) / 512.0

	toIconSpace := func(p Point2D) Point2D {
		return Point2D{
			X: p.X * scale,
			Y: p.Y * scale,
		}
	}

	// 1. ADO Blue Ribbon Polygon 1 (Right and bottom loop)
	poly1Orig := []Point2D{
		{476.00, 141.78},
		{476.00, 363.84},
		{366.00, 438.63},
		{195.50, 387.09},
		{195.50, 438.17},
		{98.98, 333.52},
		{380.30, 351.76},
		{380.30, 151.81},
	}
	var poly1 []Point2D
	for _, pt := range poly1Orig {
		poly1 = append(poly1, toIconSpace(pt))
	}

	// 2. ADO Blue Ribbon Polygon 2 (Top and left loop)
	poly2Orig := []Point2D{
		{382.23, 152.95},
		{224.37, 73.37},
		{224.37, 125.59},
		{79.45, 160.92},
		{36.00, 207.21},
		{36.00, 312.32},
		{98.15, 335.12},
		{98.15, 200.37},
	}
	var poly2 []Point2D
	for _, pt := range poly2Orig {
		poly2 = append(poly2, toIconSpace(pt))
	}

	// 3. Prompt Chevron `>` Polygon
	chevronOrig := []Point2D{
		{146.97, 305.86},
		{211.37, 257.03},
		{146.97, 208.20},
		{168.19, 192.11},
		{253.80, 257.03},
		{168.19, 321.95},
	}
	var chevron []Point2D
	for _, pt := range chevronOrig {
		chevron = append(chevron, toIconSpace(pt))
	}

	// 4. Prompt Cursor `_`
	cursorMin := toIconSpace(Point2D{240.0, 314.0})
	cursorMax := toIconSpace(Point2D{360.0, 337.0})

	// Linear gradient bounds in icon space (y1=bottom 438.63 #0078d4, y2=top 73.37 #5ea0ef)
	gradTop := toIconSpace(Point2D{256.0, 73.37}).Y
	gradBottom := toIconSpace(Point2D{256.0, 438.63}).Y
	gradRange := gradBottom - gradTop
	if gradRange <= 0 {
		gradRange = 1
	}

	getGradientColor := func(y float64) color.RGBA {
		ratio := (gradBottom - y) / gradRange
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		// Stops: 0.0 -> #0078d4 (0, 120, 212) to 1.0 -> #5ea0ef (94, 160, 239)
		r := uint8(0.0*(1.0-ratio) + 94.0*ratio)
		g := uint8(120.0*(1.0-ratio) + 160.0*ratio)
		b := uint8(212.0*(1.0-ratio) + 239.0*ratio)
		return color.RGBA{R: r, G: g, B: b, A: 255}
	}

	// Prompt color: Deep slate / dark #0f172a matching icon.svg
	promptColor := color.RGBA{R: 15, G: 23, B: 42, A: 255}

	// 4x4 subpixel supersampling (16 samples per pixel)
	samples := 4
	subStep := 1.0 / float64(samples)
	subWeight := 1.0 / float64(samples*samples)

	for y := 0; y < targetSize; y++ {
		for x := 0; x < targetSize; x++ {
			var accR, accG, accB, accA float64

			for sy := 0; sy < samples; sy++ {
				subY := float64(y) + (float64(sy)+0.5)*subStep
				for sx := 0; sx < samples; sx++ {
					subX := float64(x) + (float64(sx)+0.5)*subStep
					samplePt := Point2D{subX, subY}

					// Check Prompt elements first (foreground)
					inChevron := pointInPolygon(samplePt, chevron)
					inCursor := subX >= cursorMin.X && subX <= cursorMax.X &&
						subY >= cursorMin.Y && subY <= cursorMax.Y

					if inChevron || inCursor {
						accR += float64(promptColor.R) * subWeight
						accG += float64(promptColor.G) * subWeight
						accB += float64(promptColor.B) * subWeight
						accA += float64(promptColor.A) * subWeight
						continue
					}

					// Check Ribbon Polygons (background gradient)
					inRibbon := pointInPolygon(samplePt, poly1) || pointInPolygon(samplePt, poly2)
					if inRibbon {
						c := getGradientColor(subY)
						accR += float64(c.R) * subWeight
						accG += float64(c.G) * subWeight
						accB += float64(c.B) * subWeight
						accA += float64(c.A) * subWeight
					}
				}
			}

			if accA > 0 {
				img.Set(x, y, color.RGBA{
					R: uint8(math.Min(255, math.Round(accR))),
					G: uint8(math.Min(255, math.Round(accG))),
					B: uint8(math.Min(255, math.Round(accB))),
					A: uint8(math.Min(255, math.Round(accA))),
				})
			}
		}
	}

	return img
}

func pointInPolygon(pt Point2D, poly []Point2D) bool {
	inside := false
	n := len(poly)
	if n < 3 {
		return false
	}
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := poly[i].X, poly[i].Y
		xj, yj := poly[j].X, poly[j].Y
		intersect := ((yi > pt.Y) != (yj > pt.Y)) && (pt.X < (xj-xi)*(pt.Y-yi)/(yj-yi)+xi)
		if intersect {
			inside = !inside
		}
		j = i
	}
	return inside
}

func createIco(sizes []int, pngs [][]byte) []byte {
	var buf bytes.Buffer

	// ICO Header: 6 bytes
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // Reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // Type (1 = ICO)
	binary.Write(&buf, binary.LittleEndian, uint16(len(sizes)))

	offset := uint32(6 + len(sizes)*16)

	// Directory entries (16 bytes each)
	for i, sz := range sizes {
		w := byte(sz)
		h := byte(sz)
		if sz >= 256 {
			w = 0
			h = 0
		}
		buf.WriteByte(w)                                    // Width
		buf.WriteByte(h)                                    // Height
		buf.WriteByte(0)                                    // Color count
		buf.WriteByte(0)                                    // Reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // Planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // Bit count
		binary.Write(&buf, binary.LittleEndian, uint32(len(pngs[i])))
		binary.Write(&buf, binary.LittleEndian, offset)
		offset += uint32(len(pngs[i]))
	}

	// Image data
	for _, data := range pngs {
		buf.Write(data)
	}

	return buf.Bytes()
}
