#!/usr/bin/env bash

set -euo pipefail

mkdir -p keys trust

echo "Generating RSA private keys (dev, 2048-bit)…"
for i in 1 2 3; do
  f="keys/issuer-dev-key-$i.pem"
  if [ ! -f "$f" ]; then
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$f"
    openssl rsa -in "$f" -pubout -out "keys/issuer-dev-key-$i.pub.pem" >/dev/null 2>&1 || true
    echo "  + created $f"
  else
    echo "  = exists $f"
  fi
done

echo "Building JWKS → trust/jwks.json"
if node scripts/make-jwks.mjs; then
  echo "Done: trust/jwks.json"
else
  code=$?
  echo "ERROR: JWKS build failed; leaving existing trust/jwks.json untouched" >&2
  exit "$code"
fi

