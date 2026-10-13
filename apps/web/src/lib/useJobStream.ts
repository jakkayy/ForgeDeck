import { useEffect, useState } from "react";
import type { Job, LogLine } from "../api/types";

type Stream = { job: Job | null; lines: LogLine[]; ended: boolean };

/** Log lines that arrive together are shown together: one render per batch, not per line. */
const FLUSH_MS = 100;

/**
 * Follows a job's status and output over SSE. EventSource reconnects on its
 * own and sends Last-Event-ID, so lines are neither lost nor repeated. The
 * stream is closed when the page is left or the job changes.
 */
export function useJobStream(jobId: string): Stream {
  const [state, setState] = useState<Stream>({ job: null, lines: [], ended: false });

  useEffect(() => {
    setState({ job: null, lines: [], ended: false });
    const source = new EventSource(`/api/v1/jobs/${jobId}/events`);
    let pending: LogLine[] = [];
    let timer: ReturnType<typeof setTimeout> | undefined;

    const flush = () => {
      timer = undefined;
      if (pending.length === 0) {
        return;
      }
      const batch = pending;
      pending = [];
      setState((s) => ({ ...s, lines: s.lines.concat(batch) }));
    };

    source.addEventListener("status", (event) => {
      const job = JSON.parse((event as MessageEvent<string>).data) as Job;
      setState((s) => ({ ...s, job }));
    });
    source.addEventListener("log", (event) => {
      pending.push(JSON.parse((event as MessageEvent<string>).data) as LogLine);
      timer ??= setTimeout(flush, FLUSH_MS);
    });
    source.addEventListener("end", () => {
      source.close();
      flush();
      setState((s) => ({ ...s, ended: true }));
    });
    return () => {
      source.close();
      clearTimeout(timer);
    };
  }, [jobId]);

  return state;
}
