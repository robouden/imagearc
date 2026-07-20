# ImageArc

Open-source, cross-platform (Linux/Windows/macOS) AI photo captioning and
IPTC/XMP metadata tool for photographers. Pure Go, no CGO, single static binary.

ImageArc never rewrites your image pixels. It calls out to
[ExifTool](https://exiftool.org) to write standard IPTC/XMP metadata: in-file
for JPEG/TIFF, and Lightroom-style `.xmp` sidecars for RAW (CR2/CR3/NEF/ARW/DNG/...).

## Features

- **AI captioning** via local [Ollama](https://ollama.com) (llava, qwen2.5vl,
  qwen3-vl) — fully implemented, works offline.
- Provider interface ready for Anthropic, OpenAI, Gemini, and any
  OpenAI-compatible endpoint (stubs included, easy to fill in).
- Non-destructive metadata writes through ExifTool: caption, keywords, byline,
  location, headline, date — mapped to both IPTC and XMP fields.
- Recursive folder pipeline with a configurable worker pool and live progress.
- CSV catalog builder, with or without AI (`--no-ai` just reads existing IPTC).
- Per-client JSON templates (byline, location, keyword prefixes, caption prefix).
- Embedded dark-theme web UI: batch runner with a live activity log (SSE),
  metadata editor, and catalog builder — no external CDNs, one binary.
- No lock-in: everything lands in plain CSV, JSON, and XMP/IPTC.

## Install

Requirements:
- Go 1.22+ (to build from source)
- [ExifTool](https://exiftool.org) on your `PATH`
  - Linux: `sudo apt install libimage-exiftool-perl` (or your distro's package)
  - macOS: `brew install exiftool`
  - Windows: download from exiftool.org and add to `PATH`
- [Ollama](https://ollama.com) for local AI captioning, plus a vision model:
  ```
  ollama pull llava
  # or: ollama pull qwen2.5vl / ollama pull qwen3-vl
  ```

### Build

```
go build -o imagearc ./cmd/imagearc
```

### Cross-compile

```
GOOS=linux   GOARCH=amd64 go build -o dist/imagearc-linux-amd64     ./cmd/imagearc
GOOS=windows GOARCH=amd64 go build -o dist/imagearc-windows-amd64.exe ./cmd/imagearc
GOOS=darwin  GOARCH=arm64 go build -o dist/imagearc-macos-arm64     ./cmd/imagearc
```

## Usage

### Caption a folder

```
imagearc caption ./photos --provider ollama --model llava --recurse
```

Dry run (caption only, no metadata written):

```
imagearc caption ./photos --dry-run
```

With a client template and a custom worker count:

```
imagearc caption ./photos --recurse --template client.json --workers 4
```

`client.json`:

```json
{
  "name": "Acme Wedding Co",
  "byline": "Jane Doe Photography",
  "location": "Austin, TX",
  "keywordsPrefix": ["wedding", "acme-co"],
  "captionPrefix": "Acme Wedding Co:"
}
```

### Build a catalog

```
imagearc catalog ./photos --recurse -o catalog.csv
```

`--no-ai` is the default and only mode today: catalog reads existing IPTC/XMP
via ExifTool and writes CSV — no network, no AI calls.

### Serve the web UI

```
imagearc serve --addr localhost:8733
```

Opens your browser to a dark-themed UI for batch captioning (with a live
activity log), a metadata editor, and catalog building.

### Version

```
imagearc version
```

## How metadata is written

ImageArc maps fields to both IPTC and XMP so readers using either standard pick
them up:

| Field    | IPTC             | XMP                  |
|----------|------------------|-----------------------|
| Caption  | Caption-Abstract | dc:Description        |
| Keywords | Keywords         | dc:Subject             |
| Byline   | By-line          | dc:Creator              |
| Location | Sub-location     | iptcCore:Location        |
| Headline | Headline         | —                          |
| Date     | DateCreated      | —                          |

For RAW files, a `<file>.xmp` sidecar is created (if missing) and then updated
via ExifTool — the original RAW file is never touched. For JPEG/TIFF, ExifTool
writes in place with `-overwrite_original` (a backup is skipped since nothing
but metadata changes, and the pixel data is untouched by ExifTool tag writes).

## No lock-in

- Catalogs are plain CSV.
- Templates are plain JSON.
- Metadata lives in your files/sidecars as standard IPTC/XMP, readable by
  Lightroom, digiKam, darktable, or any other tool.

## Repositories

Mirrored on both forges — clone from whichever you prefer:

- Codeberg: https://codeberg.org/YR-Design/imagearc
- GitHub: https://github.com/robouden/imagearc

## Credits

Offline reverse geocoding (GPS → city/country) uses the
[GeoNames](https://www.geonames.org/) `cities15000` dataset, licensed
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).

## License

[CC0 1.0 Universal](LICENSE) — public domain dedication. No rights reserved.
