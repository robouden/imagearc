package metadata

import (
	"strings"
	"testing"
)

func TestIsRAW(t *testing.T) {
	cases := map[string]bool{
		"IMG_1234.CR2": true,
		"photo.nef":    true,
		"photo.dng":    true,
		"photo.jpg":    false,
		"photo.JPEG":   false,
		"photo.tiff":   false,
	}
	for path, want := range cases {
		if got := IsRAW(path); got != want {
			t.Errorf("IsRAW(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestBuildWriteArgsJPEG(t *testing.T) {
	m := Meta{
		Caption:  "a red barn at sunset",
		Keywords: []string{"barn", "sunset"},
		Byline:   "Jane Doe",
		Location: "Iowa",
		Headline: "Barn Story",
		Date:     "2024:05:01",
	}
	args := buildWriteArgs("/photos/img1.jpg", m, true)
	joined := strings.Join(args, " ")

	wantSubstrings := []string{
		"-IPTC:Caption-Abstract=a red barn at sunset",
		"-XMP-dc:Description=a red barn at sunset",
		"-IPTC:Keywords=barn",
		"-XMP-dc:Subject=barn",
		"-IPTC:Keywords=sunset",
		"-XMP-dc:Subject=sunset",
		"-IPTC:By-line=Jane Doe",
		"-XMP-dc:Creator=Jane Doe",
		"-IPTC:Sub-location=Iowa",
		"-XMP-iptcCore:Location=Iowa",
		"-IPTC:Headline=Barn Story",
		"-IPTC:DateCreated=2024:05:01",
		"-overwrite_original",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(joined, want) {
			t.Errorf("expected args to contain %q, got: %v", want, args)
		}
	}
	if args[len(args)-1] != "/photos/img1.jpg" {
		t.Errorf("expected target path as last arg, got %v", args)
	}
}

func TestBuildWriteArgsSidecarNoOverwrite(t *testing.T) {
	m := Meta{Caption: "test caption"}
	args := buildWriteArgs("/photos/img1.cr2.xmp", m, false)
	for _, a := range args {
		if a == "-overwrite_original" {
			t.Errorf("did not expect -overwrite_original when overwriteOriginal=false, got %v", args)
		}
	}
	if args[len(args)-1] != "/photos/img1.cr2.xmp" {
		t.Errorf("expected sidecar path as last arg, got %v", args)
	}
}
