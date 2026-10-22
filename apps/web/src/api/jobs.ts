import { api } from "./client";
import type { Job, JobStatus, List, LogLine } from "./types";

const finished: JobStatus[] = ["succeeded", "failed", "cancelled"];
const POLL_MS = 500;
const MAX_WAIT_MS = 120_000;

export function getJob(jobId: string, signal?: AbortSignal) {
  return api<Job>(`/jobs/${jobId}`, { signal });
}

/** Polls a job until it finishes; only while the user waits, so the short interval is bounded. */
export async function waitForJob(jobId: string, signal?: AbortSignal): Promise<Job> {
  const deadline = Date.now() + MAX_WAIT_MS;
  for (;;) {
    const job = await getJob(jobId, signal);
    if (finished.includes(job.status)) {
      return job;
    }
    if (Date.now() > deadline) {
      throw new Error("The Agent has not finished this job yet. Check that the server is online.");
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_MS));
  }
}

export async function getAllJobLogs(jobId: string, signal?: AbortSignal): Promise<LogLine[]> {
  const lines: LogLine[] = [];
  let after: string | null = null;
  for (;;) {
    const query = new URLSearchParams({ limit: "100" });
    if (after) {
      query.set("after", after);
    }
    const page: List<LogLine> = await api(`/jobs/${jobId}/logs?${query}`, { signal });
    lines.push(...page.items);
    if (page.items.length < 100 || !page.next_cursor) {
      return lines;
    }
    after = page.next_cursor;
  }
}

export type JobPage = List<Job>;

export type JobFilter = {
  status?: string;
  type?: string;
  application_id?: string;
  cursor?: string;
  limit?: number;
};

export function listJobs({ limit = 30, ...filter }: JobFilter, signal?: AbortSignal) {
  const query = new URLSearchParams({ limit: String(limit) });
  for (const [key, value] of Object.entries(filter)) {
    if (value) {
      query.set(key, value);
    }
  }
  return api<JobPage>(`/jobs?${query}`, { signal });
}

export function cancelJob(jobId: string) {
  return api<Job>(`/jobs/${jobId}/cancel`, { method: "POST" });
}
