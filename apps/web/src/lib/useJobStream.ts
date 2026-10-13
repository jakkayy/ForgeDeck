import { useEffect, useState } from "react";
import type { Job, LogLine } from "../api/types";

type Stream = { job: Job | null; lines: LogLine[]; ended: boolean };

/**
 * Follows a job's status and output over SSE. EventSource reconnects on its
 * own and sends Last-Event-ID, so lines are neither lost nor repeated.
 */
export function useJobStream(jobId: string): Stream {
  const [state, setState] = useState<Stream>({ job: null, lines: [], ended: false });

  useEffect(() => {
    setState({ job: null, lines: [], ended: false });
    const source = new EventSource(`/api/v1/jobs/${jobId}/events`);
    source.addEventListener("status", (event) => {
      const job = JSON.parse((event as MessageEvent<string>).data) as Job;
      setState((s) => ({ ...s, job }));
    });
    source.addEventListener("log", (event) => {
      const line = JSON.parse((event as MessageEvent<string>).data) as LogLine;
      setState((s) => ({ ...s, lines: [...s.lines, line] }));
    });
    source.addEventListener("end", () => {
      source.close();
      setState((s) => ({ ...s, ended: true }));
    });
    return () => source.close();
  }, [jobId]);

  return state;
}
