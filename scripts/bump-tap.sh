#!/bin/bash
# Point the Homebrew formula in a clone of kislow/homebrew-tap at a released tag, then commit and push the tap.
# Usage: TAP_DIR=/path/to/homebrew-tap bump-tap.sh vX.Y.Z
# The formula builds from the tag's source tarball, pinned by its sha256.
set -euo pipefail

readonly REPO=kislow/muxrail

die() {
  echo "bump-tap: $*" >&2
  exit 1
}

main() {
  local tag=${1:-} url sha formula
  [[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "usage: TAP_DIR=/path/to/homebrew-tap bump-tap.sh vX.Y.Z"
  [[ -n ${TAP_DIR:-} ]] || die "set TAP_DIR to a clone of kislow/homebrew-tap"
  formula="$TAP_DIR/Formula/muxrail.rb"
  [[ -f $formula ]] || die "no formula at $formula"
  url="https://github.com/$REPO/archive/refs/tags/$tag.tar.gz"
  sha=$(curl -fsSL "$url" | shasum -a 256 | cut -d' ' -f1) || die "cannot download $url; was the release PR merged?"
  git -C "$TAP_DIR" pull --quiet --ff-only
  sed -i.bak -E -e "s|^  url \".*\"|  url \"$url\"|" -e "s|^  sha256 \".*\"|  sha256 \"$sha\"|" "$formula"
  rm -f "$formula.bak"
  git -C "$TAP_DIR" diff --quiet -- Formula/muxrail.rb && die "formula already at $tag"
  git -C "$TAP_DIR" commit --quiet -m "feat(muxrail): $tag" -- Formula/muxrail.rb
  git -C "$TAP_DIR" push --quiet
  echo "tap now builds $tag; brew update && brew upgrade kislow/tap/muxrail"
}

main "$@"
