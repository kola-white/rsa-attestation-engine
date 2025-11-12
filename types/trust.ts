export type TrustCfg = {
  base_url?: string;
  jwks_path?: string;
  status_path?: string;
  latest_pointer?: string;
};

export type LatestJson = {
  sha?: string;
  prefix?: string; // "attestation-engine/<sha>"
};

export type Jwk = {
  kty: "RSA";
  use?: "sig";
  kid: string;
  alg?: "RS256";
  n: string;
  e: string;
};

export type Jwks = { keys: Jwk[] };

export type StatusEntry = { serial: string; status: "good" | "revoked" };
export type StatusList = {
  version?: string;
  ttl_s?: number;
  entries: StatusEntry[];
};

export type Policy = {
  schema_uri: string;
  allowed_assurance: string[];
};
