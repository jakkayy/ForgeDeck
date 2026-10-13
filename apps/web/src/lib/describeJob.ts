import type { Job } from "../api/types";

const short = (id: unknown, n: number) => (typeof id === "string" ? id.slice(0, n) : "");

/** A one-line, human description of what a job does. */
export function describeJob(job: Job): string {
  const p = (job.payload ?? {}) as Record<string, unknown>;
  switch (job.type) {
    case "build":
      return `Build ${String(p.repository_full_name ?? "")} @ ${short(p.commit_sha, 7)}`;
    case "container_action": {
      const action = String(p.action ?? "");
      return `${action.charAt(0).toUpperCase()}${action.slice(1)} container ${short(p.container_id, 12)}`;
    }
    case "fetch_logs":
      return `Fetch logs of container ${short(p.container_id, 12)}`;
    default:
      return job.type;
  }
}

/** "1m 12s" between two timestamps, or since start while still running. */
export function duration(start: string | null, end: string | null, now = Date.now()): string {
  if (!start) {
    return "";
  }
  const seconds = Math.max(
    0,
    Math.round(((end ? Date.parse(end) : now) - Date.parse(start)) / 1000),
  );
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
