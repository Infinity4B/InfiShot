package ocr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func TestRowsReadingOrder(t *testing.T) {
	lines := []Line{
		{Text: "world", Box: Rect{X: 0.5, Y: 0.1, W: 0.3, H: 0.1}},
		{Text: "second", Box: Rect{X: 0.1, Y: 0.5, W: 0.3, H: 0.1}},
		{Text: "hello", Box: Rect{X: 0.1, Y: 0.12, W: 0.3, H: 0.1}},
		{Text: "中文", Box: Rect{X: 0.1, Y: 0.8, W: 0.2, H: 0.1}},
		{Text: "识别", Box: Rect{X: 0.4, Y: 0.8, W: 0.2, H: 0.1}},
	}
	got := Result{Lines: lines}.Text()
	want := "hello world\nsecond\n中文识别"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestRecognizeEmpty(t *testing.T) {
	if _, err := Recognize(nil); err == nil {
		t.Fatal("Recognize(nil) succeeded")
	}
}

// TestRecognize renders text and reads it back with the system's
// recognizer. Set INFISHOT_OCR_IMAGE to a PNG to print what it reads.
func TestRecognize(t *testing.T) {
	img := renderText(t, "InfiShot OCR 2026")
	res, err := Recognize(img)
	if errors.Is(err, ErrUnsupported) {
		t.Skip("no recognizer on this system")
	}
	if err != nil {
		t.Fatal(err)
	}
	got := res.Text()
	if !strings.Contains(got, "InfiShot") || !strings.Contains(got, "2026") {
		t.Fatalf("recognized %q", got)
	}

	if path := os.Getenv("INFISHOT_OCR_IMAGE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Recognize(data)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s:\n%s", path, res.Text())
	}
}

func renderText(t *testing.T, s string) []byte {
	t.Helper()
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 48, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 640, 120))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	d := font.Drawer{Dst: img, Src: image.NewUniform(color.Black), Face: face, Dot: fixed.P(24, 76)}
	d.DrawString(s)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCharBoxes(t *testing.T) {
	// Estimated: "中a" splits 2:1.
	l := Line{Text: "中a", Box: Rect{X: 0.1, Y: 0.2, W: 0.3, H: 0.1}}
	got := l.CharBoxes()
	if len(got) != 2 || math.Abs(got[0].W-0.2) > 1e-9 || math.Abs(got[1].X-0.3) > 1e-9 {
		t.Fatalf("estimated %+v", got)
	}
}
