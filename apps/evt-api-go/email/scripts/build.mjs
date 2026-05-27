import fs from "node:fs";
import path from "node:path";
import mjml2html from "mjml";

const srcRoot = path.resolve("src/templates");
const distRoot = path.resolve("dist");

const walk = (dir) => {
  const entries = fs.readdirSync(dir, { withFileTypes: true });

  return entries.flatMap((entry) => {
    const fullPath = path.join(dir, entry.name);

    if (entry.isDirectory()) {
      return walk(fullPath);
    }

    return [fullPath];
  });
};

fs.rmSync(distRoot, { recursive: true, force: true });

for (const filePath of walk(srcRoot)) {
  const relativePath = path.relative(srcRoot, filePath);
  const parsed = path.parse(relativePath);

  if (filePath.endsWith(".mjml")) {
    const mjmlSource = fs.readFileSync(filePath, "utf8");

    const result = mjml2html(mjmlSource, {
      filePath,
      validationLevel: "strict",
      minify: true,
    });

    const outputRelativePath = path.join(
      parsed.dir,
      `${parsed.name}.gotmpl`,
    );

    const outputPath = path.join(distRoot, outputRelativePath);

    fs.mkdirSync(path.dirname(outputPath), { recursive: true });
    fs.writeFileSync(outputPath, result.html, "utf8");

    console.log(`compiled ${relativePath} -> ${path.relative(".", outputPath)}`);
    continue;
  }

  if (filePath.endsWith(".gotmpl")) {
    const outputPath = path.join(distRoot, relativePath);

    fs.mkdirSync(path.dirname(outputPath), { recursive: true });
    fs.copyFileSync(filePath, outputPath);

    console.log(`copied ${relativePath} -> ${path.relative(".", outputPath)}`);
  }
}