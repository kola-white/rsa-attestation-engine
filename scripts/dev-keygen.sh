set -euo pipefail

mkdir -p keys trust

echo "Generating RSA private keys (dev, 2048-bit)…"
for i in 1 2 3; do
  f="keys/issuer-dev-key-$i.pem"
  if [ ! -f "$f" ]; then
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$f"
    # optional: export public PEM for manual inspection
    openssl rsa -in "$f" -pubout -out "keys/issuer-dev-key-$i.pub.pem" >/dev/null 2>&1 || true
    echo "  + created $f"
  else
    echo "  = exists $f"
  fi
done

echo "Building JWKS → trust/jwks.json"
tmp="$(mktemp)"
keys=(keys/issuer-dev-key-{1..3}.pem)
if node scripts/pem-to-jwks.mjs "${keys[@]}" > "$tmp"; then
  mv "$tmp" trust/jwks.json
  echo "Done: trust/jwks.json"
else
  code=$?
  echo "ERROR: converter failed; leaving existing trust/jwks.json untouched" >&2
  rm -f "$tmp"
  exit "$code"
fi
