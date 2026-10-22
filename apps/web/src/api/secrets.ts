import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";

export type Secret = {
  name: string;
  version: number;
  created_at: string;
  updated_at: string;
};

export const secretsQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId, "secrets"],
    queryFn: ({ signal }) =>
      api<{ items: Secret[] }>(`/projects/${projectId}/secrets`, { signal }).then((r) => r.items),
  });

/** Creates the secret or replaces its value; the value is never read back. */
export function setSecret(projectId: string, name: string, value: string) {
  return api<Secret>(`/projects/${projectId}/secrets/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: { value },
  });
}

export function deleteSecret(projectId: string, name: string) {
  return api<void>(`/projects/${projectId}/secrets/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}
