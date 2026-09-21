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
  throw new Error(
    `GITHUB_SHA must be a full 40-character Git SHA: ${GITHUB_SHA}`,
  );
}

if (!/^\d{4}-\d{2}-\d{2}$/.test(CI_EVIDENCE_DATE)) {
  throw new Error(
    `CI_EVIDENCE_DATE must use YYYY-MM-DD format: ${CI_EVIDENCE_DATE}`,
  );
}

const source = ".ci-evidence/deployment.json";
const destination =
  `ci-evidence/${CI_EVIDENCE_DATE}/${GITHUB_SHA}/deployment.json`;

const s3 = new S3Client({
  region: DO_REGION,
  endpoint: DO_ENDPOINT,
  credentials: {
    accessKeyId: DO_SPACES_KEY,
    secretAccessKey: DO_SPACES_SECRET,
  },
});

const body = await readFile(source);

await s3.send(
  new PutObjectCommand({
    Bucket: DO_SPACE,
    Key: destination,
    Body: body,
    ContentType: "application/json",
    CacheControl: "no-cache",
    IfNoneMatch: "*",
  }),
);

console.log(`Archived ${destination}`);
console.log(`Deployment evidence archived for ${GITHUB_SHA}.`);