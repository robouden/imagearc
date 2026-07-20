# ImageArc + Shotwell (and other photo managers)

ImageArc is an **AI captioning engine**: it writes captions and keywords into
each photo as standard **IPTC/XMP** metadata. Any library manager that reads
embedded metadata — Shotwell, digiKam, Lightroom, darktable — then picks those
up as tags. No plugin required (and Shotwell's plugin API only supports web
publishing and slideshow transitions anyway, so a captioning plugin isn't
possible there).

## Right-click captioning (GNOME Files / Nemo / Caja)

Install the file-manager action:

```sh
./integrations/install-nautilus.sh            # Nautilus / GNOME Files
./integrations/install-nautilus.sh nemo       # Cinnamon
./integrations/install-nautilus.sh caja       # MATE
nautilus -q                                    # restart the file manager
```

Then select photos or a folder → **right-click → Scripts → Caption with
ImageArc**. A terminal window opens showing live per-file progress; captions and
keywords are written into the files, and a notification fires when it finishes.

Override the model/provider per your setup (otherwise the defaults in
`~/.config/imagearc/config.json` are used):

```sh
IMAGEARC_MODEL=qwen3-vl IMAGEARC_PROVIDER=ollama  # e.g. in the script or your env
```

## Seeing the tags in Shotwell

1. **Edit → Preferences →** enable *"Write tags, titles, and other metadata to
   photo files"* (lets Shotwell round-trip embedded metadata).
2. Shotwell reads embedded keywords **on import**. For already-imported photos
   it doesn't always re-read external changes, so **re-import** the folder
   (**File → Import From Folder…**) — Shotwell dedupes, so this is safe. The AI
   keywords then appear as **tags** you can search, filter, and sort by.

## Why this instead of a Shotwell plugin

Shotwell's plugin system (SPIT, Vala) exposes only *publishing services* and
*slideshow transitions* — there is no extension point for processing photos,
writing metadata, or adding menu actions. Standard IPTC/XMP is the supported,
lock-in-free integration path, and it works with every major photo manager.
