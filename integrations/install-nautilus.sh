#!/usr/bin/env bash
# Install the "Caption with ImageArc" right-click action into your file manager.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
src="$here/nautilus/Caption with ImageArc"

# Pick the scripts dir for the detected file manager (default: Nautilus).
case "${1:-nautilus}" in
  nemo) dest="${XDG_DATA_HOME:-$HOME/.local/share}/nemo/scripts" ;;
  caja) dest="${XDG_CONFIG_HOME:-$HOME/.config}/caja/scripts" ;;
  *)    dest="${XDG_DATA_HOME:-$HOME/.local/share}/nautilus/scripts" ;;
esac

mkdir -p "$dest"
install -m 755 "$src" "$dest/Caption with ImageArc"

case "${1:-nautilus}" in
  nemo) quit="nemo -q" ;;
  caja) quit="caja -q" ;;
  *)    quit="nautilus -q" ;;
esac
echo "Installed to: $dest"
echo "Restart the file manager ('$quit'), then right-click photos/folders"
echo "→ Scripts → Caption with ImageArc."
