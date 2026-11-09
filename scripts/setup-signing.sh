#!/bin/zsh
# 创建 InfiShot 的本地签名证书（自签名，只在本机使用），只需运行一次。
#
# 用固定证书签名后，每次重新编译的 InfiShot 在系统看来都是同一个应用，
# 「屏幕录制」授权不会因为重新编译而失效。证书放在单独的钥匙串里，
# 钥匙串的随机密码存在登录钥匙串中，构建时自动解锁，不会弹窗，
# 也不需要修改系统的证书信任设置（codesign 不要求证书受信任）。
set -euo pipefail
source "${0:A:h}/signing.env"

if [[ -f "$SIGN_KEYCHAIN" ]] && security find-identity -p codesigning "$SIGN_KEYCHAIN" 2>/dev/null | grep -q "\"$SIGN_NAME\""; then
  echo "签名证书已存在：$SIGN_NAME"
  exit 0
fi
if [[ -f "$SIGN_KEYCHAIN" ]]; then
  security delete-keychain "$SIGN_KEYCHAIN" # an earlier run that did not finish
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/cert.cnf" <<CNF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $SIGN_NAME
O = InfiShot Development
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
CNF
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -config "$tmp/cert.cnf" \
  -keyout "$tmp/key.pem" -out "$tmp/cert.pem" 2>/dev/null
p12pass=$(openssl rand -hex 16)
# macOS imports only PKCS#12 files in the legacy algorithms; OpenSSL 3
# needs -legacy for them, LibreSSL has no such option and makes them.
openssl pkcs12 -export -legacy -inkey "$tmp/key.pem" -in "$tmp/cert.pem" -out "$tmp/id.p12" -passout "pass:$p12pass" 2>/dev/null ||
  openssl pkcs12 -export -inkey "$tmp/key.pem" -in "$tmp/cert.pem" -out "$tmp/id.p12" -passout "pass:$p12pass"

kcpass=$(openssl rand -hex 24)
security create-keychain -p "$kcpass" "$SIGN_KEYCHAIN"
security set-keychain-settings "$SIGN_KEYCHAIN" # never locks by itself
security add-generic-password -U -a "$USER" -s "$SIGN_SERVICE" -l "InfiShot 签名钥匙串" -w "$kcpass"
security unlock-keychain -p "$kcpass" "$SIGN_KEYCHAIN"
security import "$tmp/id.p12" -k "$SIGN_KEYCHAIN" -P "$p12pass" -T /usr/bin/codesign >/dev/null
# Lets codesign use the key without asking.
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$kcpass" "$SIGN_KEYCHAIN" >/dev/null

# In the keychains codesign searches, after those already there.
keychains=("${(@f)$(security list-keychains -d user | sed -e 's/^[[:space:]]*"//' -e 's/"$//')}")
security list-keychains -d user -s "${keychains[@]}" "$SIGN_KEYCHAIN"

echo "已创建签名证书：$SIGN_NAME"
security find-identity -p codesigning "$SIGN_KEYCHAIN" | grep "$SIGN_NAME"
