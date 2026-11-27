// Artifact-only distribution. Never reads production environment credentials or
// imports the existing automatic Spaces publisher. No private database access.
import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { inspectPublication } from "./verify.mjs";
const maxBytes = 1048576;
const unavailable = () => ({
  trustMaterial: { availability: "UNAVAILABLE" },
  statusMaterial: { availability: "UNAVAILABLE" },
});
function bounded(file) {
  const st = fs.lstatSync(file);
  if (!st.isFile() || st.isSymbolicLink() || st.size > maxBytes)
    throw Error("unsafe artifact");
  return fs.readFileSync(file, "utf8");
}
function pointer(dir) {
  const p = JSON.parse(bounded(path.join(dir, "latest.json")));
  if (
    Object.keys(p).sort().join() !== "statusFile,trustFile" ||
    !/^versions\/trust-[1-9][0-9]*\.jwt$/.test(p.trustFile) ||
    !/^versions\/status-[1-9][0-9]*\.jwt$/.test(p.statusFile)
  )
    throw Error("invalid pointer");
  const versions = path.join(dir, "versions");
  if (fs.lstatSync(versions).isSymbolicLink()) throw Error("symlink");
  return p;
}
export function receive(dir) {
  try {
    const p = pointer(dir);
    return {
      trustMaterial: {
        availability: "AVAILABLE",
        jws: bounded(path.join(dir, p.trustFile)),
      },
      statusMaterial: {
        availability: "AVAILABLE",
        jws: bounded(path.join(dir, p.statusFile)),
      },
    };
  } catch (e) {
    if (e.code === "ENOENT" && !fs.existsSync(path.join(dir, "latest.json")))
      return unavailable();
    return {
      trustMaterial: { availability: "INVALID" },
      statusMaterial: { availability: "INVALID" },
    };
  }
}
function immutable(file, body) {
  try {
    const fd = fs.openSync(file, "wx", 0o644);
    try {
      fs.writeFileSync(fd, body);
      fs.fsyncSync(fd);
    } finally {
      fs.closeSync(fd);
    }
  } catch (e) {
    if (e.code !== "EEXIST" || bounded(file) !== body) throw e;
  }
}
export async function publish(dir, packet, anchor, policy) {
  fs.mkdirSync(dir, { recursive: true });
  if (fs.lstatSync(dir).isSymbolicLink())
    throw Error("unsafe publication directory");
  const lock = path.join(dir, ".publish.lock");
  const fd = fs.openSync(lock, "wx", 0o600);
  let temp,
    renameAttempted = false;
  const annotate = (e) => {
    e.commitMayHaveOccurred = renameAttempted;
    return e;
  };
  try {
    const trust = packet.trustMaterial.jws,
      status = packet.statusMaterial.jws;
    const next = await inspectPublication(trust, status, anchor, policy);
    const old = receive(dir);
    if (old.trustMaterial.availability === "INVALID")
      throw Error("existing publication corrupt");
    if (old.trustMaterial.availability === "AVAILABLE") {
      const previous = await inspectPublication(
        old.trustMaterial.jws,
        old.statusMaterial.jws,
        anchor,
        policy,
        true,
      );
      if (
        next.trustVersion < previous.trustVersion ||
        next.statusVersion < previous.statusVersion ||
        next.trustIat < previous.trustIat ||
        next.statusIat < previous.statusIat ||
        (next.trustVersion === previous.trustVersion &&
          trust !== old.trustMaterial.jws) ||
        (next.statusVersion === previous.statusVersion &&
          status !== old.statusMaterial.jws)
      )
        throw Error("publication rollback or equivocation");
    }
    const versions = path.join(dir, "versions");
    fs.mkdirSync(versions, { recursive: true });
    if (fs.lstatSync(versions).isSymbolicLink())
      throw Error("unsafe versions directory");
    const p = {
      trustFile: `versions/trust-${next.trustVersion}.jwt`,
      statusFile: `versions/status-${next.statusVersion}.jwt`,
    };
    immutable(path.join(dir, p.trustFile), trust);
    immutable(path.join(dir, p.statusFile), status);
    temp = path.join(dir, ".latest-" + randomUUID());
    const tf = fs.openSync(temp, "wx", 0o644);
    try {
      fs.writeFileSync(tf, JSON.stringify(p) + "\n");
      fs.fsyncSync(tf);
    } finally {
      fs.closeSync(tf);
    }
    renameAttempted = true;
    fs.renameSync(temp, path.join(dir, "latest.json"));
    temp = null;
    const d = fs.openSync(dir, "r");
    try {
      fs.fsyncSync(d);
    } finally {
      fs.closeSync(d);
    }
    return { ...next, committed: true };
  } catch (e) {
    throw annotate(e);
  } finally {
    try {
      if (temp && fs.existsSync(temp)) fs.unlinkSync(temp);
      fs.closeSync(fd);
      fs.unlinkSync(lock);
    } catch (e) {
      throw annotate(e);
    }
  }
}
// Only operator-configured HTTPS bulk URLs are fetched. No credential-provided
// URI, credential index, cookie, authorization token or redirect is followed.
export async function acquire(config, fetcher = fetch) {
  async function one(url, accept) {
    try {
      const u = new URL(url);
      if (
        u.protocol !== "https:" ||
        u.username ||
        u.password ||
        u.search ||
        u.hash
      )
        throw Error("unsafe URL");
    } catch {
      return { availability: "INVALID" };
    }
    try {
      const r = await fetcher(url, {
        redirect: "error",
        credentials: "omit",
        headers: { Accept: accept },
        signal: AbortSignal.timeout(5000),
      });
      if (!r.ok) return { availability: "UNAVAILABLE" };
      if (r.headers.get("content-type")?.split(";")[0] !== accept)
        throw Error("media type");
      let size = 0;
      const chunks = [];
      for await (const b of r.body) {
        size += b.length;
        if (size > maxBytes) throw Error("size");
        chunks.push(b);
      }
      return {
        availability: "AVAILABLE",
        jws: Buffer.concat(chunks).toString("utf8"),
      };
    } catch (e) {
      return {
        availability:
          e.name === "TimeoutError" || e.name === "TypeError"
            ? "UNAVAILABLE"
            : "INVALID",
      };
    }
  }
  return {
    trustMaterial: await one(config.trustURL, "application/jwt"),
    statusMaterial: await one(config.statusURL, "application/statuslist+jwt"),
  };
}
