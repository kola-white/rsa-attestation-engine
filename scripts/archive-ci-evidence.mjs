import { readFile } from "node:fs/promises";
import { S3Client, PutObjectCommand } from "@aws-sdk/client-s3";

const requiredEnv = [
  "DO_SPACES_KEY",
  "DO_SPACES_SECRET",
  "DO_SPACE",
  "DO_REGION",
  "DO_ENDPOINT",
  "GITHUB_SHA",
  "CI_EVIDENCE_DATE",
];

for (const name of requiredEnv) {
  if (!process.env[name]) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
}

const {
  DO_SPACES_KEY,
  DO_SPACES_SECRET,
  DO_SPACE,
  DO_REGION,
  DO_ENDPOINT,
  GITHUB_SHA,
  CI_EVIDENCE_DATE,
} = process.env;

if (!/^[0-9a-f]{40}$/i.test(GITHUB_SHA)) {
  throw new Error(`GITHUB_SHA must be a full 40-character Git SHA: ${GITHUB_SHA}`);
}

if (!/^\d{4}-\d{2}-\d{2}$/.test(CI_EVIDENCE_DATE)) {
  throw new Error(
    `CI_EVIDENCE_DATE must use YYYY-MM-DD format: ${CI_EVIDENCE_DATE}`,
  );
}

const evidenceDir = ".ci-evidence";
const prefix = `ci-evidence/${CI_EVIDENCE_DATE}/${GITHUB_SHA}`;

const files = [
  {
    source: `${evidenceDir}/manifest.json`,
    destination: `${prefix}/manifest.json`,
    contentType: "application/json",
  },
  {
    source: `${evidenceDir}/node-tests.txt`,
    destination: `${prefix}/node-tests.txt`,
    contentType: "text/plain; charset=utf-8",
  },
  {
    source: `${evidenceDir}/go-tests.txt`,
    destination: `${prefix}/go-tests.txt`,
    contentType: "text/plain; charset=utf-8",
  },
];

const s3 = new S3Client({
  region: DO_REGION,
  endpoint: DO_ENDPOINT,
  credentials: {
    accessKeyId: DO_SPACES_KEY,
    secretAccessKey: DO_SPACES_SECRET,
  },
});

for (const file of files) {
  const body = await readFile(file.source);

  await s3.send(
    new PutObjectCommand({
      Bucket: DO_SPACE,
      Key: file.destination,
      Body: body,
      ContentType: file.contentType,
      CacheControl: "no-cache",
      IfNoneMatch: "*",
    }),
  );

  console.log(`Archived ${file.destination}`);
}

console.log(
  `CI evidence archived for ${GITHUB_SHA} under ${prefix}/`,
);
