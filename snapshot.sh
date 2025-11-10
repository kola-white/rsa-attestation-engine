mkdir -p reports

npx --yes cloc . --json > reports/cloc.json
npx --yes cloc . > reports/cloc.txt

npx --yes dependency-cruiser src --no-config --output-type dot | dot -Tpng -o reports/dep-graph.png

git log --since="30 days ago" --pretty="%ad %h %s" --date=short > reports/commits-30d.txt

tree -L 2 -I 'node_modules|dist|build|coverage|.git' > reports/tree.txt

if [ -d public ]; then
  if command -v shasum >/dev/null 2>&1; then HASHSUM="shasum -a 256"; else HASHSUM="sha256sum"; fi
  find public -type f -name "*.json" -print0 | xargs -0 $HASHSUM | awk '{print "{\"path\":\""$2"\",\"sha256\":\""$1"\"}"}' | jq -s '.' > reports/public-manifest.json
fi

