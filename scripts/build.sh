#!/bin/zsh
# 用本地签名证书打包 InfiShot（build/darwin-arm64/InfiShot.app 和 .dmg）。
#   scripts/build.sh       打包
#   scripts/build.sh -i    打包后安装到 /Applications 并重新打开
# 其余参数交给 go tool mygo build。第一次使用前运行 scripts/setup-signing.sh。
set -euo pipefail
cd "${0:A:h}/.."
source scripts/signing.env

install=false
if [[ "${1:-}" == "-i" ]]; then
  install=true
  shift
fi
if [[ ! -f "$SIGN_KEYCHAIN" ]]; then
  echo "没有找到签名钥匙串，先运行 scripts/setup-signing.sh" >&2
  exit 1
fi
security unlock-keychain -p "$(security find-generic-password -s "$SIGN_SERVICE" -w)" "$SIGN_KEYCHAIN"
go tool mygo build -sign "$SIGN_NAME" "$@"

if $install; then
  app=build/darwin-arm64/InfiShot.app
  pkill -x InfiShot 2>/dev/null && sleep 0.5 || true
  rm -rf /Applications/InfiShot.app
  ditto "$app" /Applications/InfiShot.app
  open /Applications/InfiShot.app
  echo "已安装并打开 /Applications/InfiShot.app"
fi
