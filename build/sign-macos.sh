#!/bin/bash
set -euo pipefail

[[ "$(uname -s)" == Darwin ]] || { echo 'Signing requires macOS.' >&2; exit 1; }
: "${MACOS_SIGN_IDENTITY:?Set the Developer ID Application identity name}"
: "${MACOS_NOTARY_PROFILE:?Set the notarytool keychain profile name}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PACKAGE="$ROOT/dist/wx-mini-video-macos"
APP="$PACKAGE/wx-mini-video.app"
[[ -f "$APP/Contents/MacOS/wx-mini-video" ]] || { echo 'Run go run ./build/macos first.' >&2; exit 1; }
for item in ffmpeg/ffmpeg-amd64 ffmpeg/ffmpeg-arm64 ffmpeg/LICENSE.txt THIRD-PARTY-NOTICES.md; do
    [[ -f "$PACKAGE/$item" && ! -L "$PACKAGE/$item" ]] || { echo "Missing or symlinked package file: $item" >&2; exit 1; }
done
python3 "$ROOT/build/verify-release-sources.py"
[[ -z "$(find "$APP" -type l -print -quit)" ]] || { echo 'Unexpected symlink inside app.' >&2; exit 1; }
while IFS= read -r -d '' item; do
    case "${item#"$APP"/}" in
        Contents/Info.plist|Contents/MacOS/wx-mini-video|Contents/_CodeSignature/CodeResources) ;;
        *) echo "Unexpected file inside app: $item" >&2; exit 1 ;;
    esac
done < <(find "$APP" -type f -print0)

codesign --force --options runtime --timestamp --entitlements "$ROOT/build/macos-entitlements.plist" --sign "$MACOS_SIGN_IDENTITY" "$APP"
codesign --verify --deep --strict "$APP"
WORK="$(mktemp -d "$ROOT/dist/.macos-sign-XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
ditto -c -k --keepParent "$APP" "$WORK/notary.zip"
xcrun notarytool submit "$WORK/notary.zip" --keychain-profile "$MACOS_NOTARY_PROFILE" --wait
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"
spctl --assess --type execute --verbose "$APP"

mkdir "$WORK/wx-mini-video-macos"
ditto "$APP" "$WORK/wx-mini-video-macos/wx-mini-video.app"
cp "$ROOT/docs/macos-usage.md" "$WORK/wx-mini-video-macos/README.md"
cp "$ROOT/internal/config/config.template.yaml" "$WORK/wx-mini-video-macos/wx-mini-video.yaml"
mkdir "$WORK/wx-mini-video-macos/ffmpeg"
for item in ffmpeg/ffmpeg-amd64 ffmpeg/ffmpeg-arm64 ffmpeg/LICENSE.txt; do
    ditto "$PACKAGE/$item" "$WORK/wx-mini-video-macos/$item"
done
cp "$PACKAGE/THIRD-PARTY-NOTICES.md" "$WORK/wx-mini-video-macos/THIRD-PARTY-NOTICES.md"
ditto -c -k --keepParent "$WORK/wx-mini-video-macos" "$ROOT/dist/wx-mini-video-macos-universal.zip"
for item in ffmpeg/ffmpeg-amd64 ffmpeg/ffmpeg-arm64 ffmpeg/LICENSE.txt THIRD-PARTY-NOTICES.md; do
    unzip -p "$ROOT/dist/wx-mini-video-macos-universal.zip" "wx-mini-video-macos/$item" | cmp - "$PACKAGE/$item"
done
echo 'Signed and notarized package created. Real WeChat acceptance is still required.'
