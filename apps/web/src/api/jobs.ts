import { api } from "./client";
import type { Job, JobStatus } from "./types";

const finished: JobStatus[] = ["succeeded", "failed", "cancelled"];
const POLL_MS = 500;
const MAX_WAIT_MS = 120_000;

export function getJob(jobId: string, signal?: AbortSignal) {
  return api<Job>(`/jobs/${jobId}`, { signal });
}

/**
 * Polls a job until it finishes. Used only while the user waits on an action
 * they just started, so the short interval is bounded in time.
 */
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
