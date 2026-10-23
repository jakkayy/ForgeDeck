import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";

export type RunnerStatus = "pending" | "online" | "offline";

export type Runner = {
  id: string;
  name: string;
  version: string | null;
  status: RunnerStatus;
  last_seen_at: string | null;
  created_at: string;
};

/** What creating a Runner returns: the credential is shown once and never again. */
export type CreatedRunner = Runner & { credential: string };

/** Matches the Runner heartbeat, so status is never much more than one beat behind. */
const REFRESH_MS = 10_000;

export const runnersQuery = queryOptions({
  queryKey: ["runners"],
  queryFn: ({ signal }) => api<{ items: Runner[] }>("/runners", { signal }).then((r) => r.items),
  refetchInterval: REFRESH_MS,
});

export function createRunner(name: string) {
  return api<CreatedRunner>("/runners", { method: "POST", body: { name } });
}

export function deleteRunner(id: string) {
  return api<void>(`/runners/${id}`, { method: "DELETE" });
}
