import type { ServiceConfig } from "../api/environments";

/** "KEY=value" lines to a map. Blank lines and lines starting with # are ignored. */
export function parseEnv(text: string): Record<string, string> {
  const env: Record<string, string> = {};
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (line === "" || line.startsWith("#")) {
      continue;
    }
    const at = line.indexOf("=");
    if (at > 0) {
      env[line.slice(0, at).trim()] = line.slice(at + 1);
    } else {
      // Keep the malformed line as a name so the server reports it on this field.
      env[line] = "";
    }
  }
  return env;
}

export function envToText(env: Record<string, string>): string {
  return Object.entries(env)
    .map(([name, value]) => `${name}=${value}`)
    .join("\n");
}

/** "name:/path" lines to volume mounts. */
export function parseVolumes(text: string): ServiceConfig["volumes"] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "")
    .map((line) => {
      const at = line.indexOf(":");
      return at < 0
        ? { name: line, path: "" }
        : { name: line.slice(0, at), path: line.slice(at + 1) };
    });
}

export function volumesToText(volumes: ServiceConfig["volumes"]): string {
  return volumes.map((v) => `${v.name}:${v.path}`).join("\n");
}

/** An empty field is "not set"; anything else is a number the server validates. */
export function parseOptionalNumber(value: string): number | null {
  const trimmed = value.trim();
  return trimmed === "" ? null : Number(trimmed);
}

/** "ghcr.io/acme/api@sha256:0123…" to "api@0123456789ab" for compact display. */
export function shortImage(ref: string): string {
  const [name, digest] = ref.split("@");
  const base = name?.split("/").pop() ?? ref;
  return digest ? `${base}@${digest.replace("sha256:", "").slice(0, 12)}` : base;
}
