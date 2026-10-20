#!/bin/bash
set -euo pipefail

[[ "$(uname -s)" == Darwin ]] || { echo 'Signing requires macOS.' >&2; exit 1; }
: "${MACOS_SIGN_IDENTITY:?Set the Developer ID Application identity name}"
: "${MACOS_NOTARY_PROFILE:?Set the notarytool keychain profile name}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PACKAGE="$ROOT/dist/wx-mini-video-macos"
APP="$PACKAGE/wx-mini-video.app"
[[ -f "$APP/Contents/MacOS/wx-mini-video" ]] || { echo 'Run go run ./build/macos first.' >&2; exit 1; }
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
ditto -c -k --keepParent "$WORK/wx-mini-video-macos" "$ROOT/dist/wx-mini-video-macos-universal.zip"
echo 'Signed and notarized package created. Real WeChat acceptance is still required.'
