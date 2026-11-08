// Command lamp-palette checks a HomePod ambient-light palette the way the
// receiver does: it builds the flat three-band carrier, runs the local Lab
// k-means, and prints the recovered colors, pairwise distances, and the
// approximate device stops. The top display is the final judge; the stops
// here use the scale-by-max normalization model.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"

	"saundo/internal/lamp"
)

type hexColors []color.RGBA

func (h *hexColors) String() string { return fmt.Sprint([]color.RGBA(*h)) }

func (h *hexColors) Set(v string) error {
	c, err := parseHex(v)
	if err != nil {
		return err
	}
	*h = append(*h, c)
	return nil
}

func parseHex(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("want #RRGGBB, got %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}

func parseWeights(s string) ([3]float64, error) {
	var out [3]float64
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return out, fmt.Errorf("want three comma-separated percentages, got %q", s)
	}
	sum := 0.0
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || v <= 0 {
			return out, fmt.Errorf("invalid weight %q", p)
		}
		out[i] = v / 100
		sum += out[i]
	}
	if math.Abs(sum-1) > 1e-6 {
		return out, fmt.Errorf("weights sum to %.1f%%, want 100%%", sum*100)
	}
	return out, nil
}

// carrier matches the production carrier: flat bands with a narrow feather.
func carrier(colors [3]color.RGBA, weights [3]float64) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, lamp.Size, lamp.Size))
	var cuts [4]int
	for i, w := range weights {
		cuts[i+1] = cuts[i] + int(w*lamp.Size+0.5)
	}
	cuts[3] = lamp.Size
	const feather = lamp.Size / 24
	for y := 0; y < lamp.Size; y++ {
		for x := 0; x < lamp.Size; x++ {
			img.SetRGBA(x, y, carrierColor(colors, cuts, feather, x))
		}
	}
	return img
}

func carrierColor(colors [3]color.RGBA, cuts [4]int, feather, x int) color.RGBA {
	for i := 0; i < 2; i++ {
		cut := cuts[i+1]
		if x < cut-feather {
			return colors[i]
		}
		if x <= cut+feather {
			t := float64(x-(cut-feather)) / float64(2*feather)
			return blend(colors[i], colors[i+1], t)
		}
	}
	return colors[2]
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t + 0.5)
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xff}
}

func lab(c color.RGBA) (float64, float64, float64) {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.R), lin(c.G), lin(c.B)
	x := (0.4124564*r + 0.3575761*g + 0.1804375*b) / 0.95047
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := (0.0193339*r + 0.1191920*g + 0.9503041*b) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

func deltaE(a, b color.RGBA) float64 {
	l1, a1, b1 := lab(a)
	l2, a2, b2 := lab(b)
	return math.Sqrt((l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
}

// deviceStop models the receiver's color-stop normalization: scale so the
// brightest channel reaches 1.0, with a 1e-4 floor, then clamp.
func deviceStop(c color.RGBA) color.RGBA {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	m := math.Max(r, math.Max(g, b))
	if m < 1e-4 {
		m = 1e-4
	}
	s := 1 / m
	cl := func(v float64) uint8 {
		v *= s
		if v > 1 {
			v = 1
		}
		return uint8(v*255 + 0.5)
	}
	return color.RGBA{R: cl(r), G: cl(g), B: cl(b), A: 0xff}
}

// dim models the renderer's brightness curve, roughly 0.7 to 0.2 plus a
// 0.015 floor.
func dim(c color.RGBA) color.RGBA {
	cl := func(v uint8) uint8 {
		f := float64(v)/255*0.286 + 0.015
		if f > 1 {
			f = 1
		}
		return uint8(f*255 + 0.5)
	}
	return color.RGBA{R: cl(c.R), G: cl(c.G), B: cl(c.B), A: 0xff}
}

func hex(c color.RGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func main() {
	var colors hexColors
	flag.Var(&colors, "c", "hex color, repeat three times")
	weightsFlag := flag.String("w", "45,30,25", "area weights in percent")
	presetFlag := flag.String("preset", "", "analyze an existing preset ID")
	flag.Parse()

	var p lamp.Preset
	if *presetFlag != "" {
		if len(colors) != 0 {
			fmt.Fprintln(os.Stderr, "use either -preset or -c, not both")
			os.Exit(2)
		}
		var ok bool
		p, ok = lamp.ByID(*presetFlag)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown preset %q\n", *presetFlag)
			os.Exit(2)
		}
	} else {
		if len(colors) != 3 {
			fmt.Fprintln(os.Stderr, "give three -c colors, or -preset <id>")
			flag.Usage()
			os.Exit(2)
		}
		w, err := parseWeights(*weightsFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		p = lamp.Preset{
			ID:      "candidate",
			Colors:  [3]color.RGBA{colors[0], colors[1], colors[2]},
			Weights: w,
		}
	}

	got, ok := lamp.CoverPreset(carrier(p.Colors, p.Weights))
	if !ok {
		fmt.Fprintln(os.Stderr, "CoverPreset returned no palette")
		os.Exit(1)
	}

	fmt.Printf("%-7s target  %s %s %s  weights %.0f/%.0f/%.0f\n", p.ID,
		hex(p.Colors[0]), hex(p.Colors[1]), hex(p.Colors[2]),
		p.Weights[0]*100, p.Weights[1]*100, p.Weights[2]*100)
	fmt.Printf("%-7s k-means %s %s %s\n", "", hex(got.Colors[0]), hex(got.Colors[1]), hex(got.Colors[2]))
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			fmt.Printf("%-7s dE(%s,%s)=%.1f\n", "", hex(p.Colors[i]), hex(p.Colors[j]), deltaE(p.Colors[i], p.Colors[j]))
		}
	}
	fmt.Printf("%-7s device  %s %s %s  (scale-by-max approximation)\n", "",
		hex(deviceStop(p.Colors[0])), hex(deviceStop(p.Colors[1])), hex(deviceStop(p.Colors[2])))
	fmt.Printf("%-7s dimmed  %s %s %s  (0.7 -> 0.2 curve, approximation)\n", "",
		hex(dim(deviceStop(p.Colors[0]))), hex(dim(deviceStop(p.Colors[1]))), hex(dim(deviceStop(p.Colors[2]))))

	fail := false
	for _, want := range p.Colors {
		best := math.Inf(1)
		for _, c := range got.Colors {
			if d := deltaE(want, c); d < best {
				best = d
			}
		}
		if best > 12 {
			fmt.Printf("%-7s FAIL    %s is %.1f from the recovered palette\n", "", hex(want), best)
			fail = true
		}
	}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if d := deltaE(p.Colors[i], p.Colors[j]); d < 12 {
				fmt.Printf("%-7s FAIL    %s and %s are only %.1f apart\n", "", hex(p.Colors[i]), hex(p.Colors[j]), d)
				fail = true
			}
		}
	}
	if fail {
		os.Exit(1)
	}
	fmt.Printf("%-7s PASS    every color recovered; every pair at least dE 12\n", "")
}
