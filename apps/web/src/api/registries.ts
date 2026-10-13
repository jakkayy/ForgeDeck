import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";

export type Registry = {
  id: string;
  project_id: string;
  name: string;
  host: string;
  username: string;
  created_at: string;
};

export type NewRegistry = { name: string; host: string; username: string; password: string };

export const registriesQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId, "registries"],
    queryFn: ({ signal }) =>
      api<{ items: Registry[] }>(`/projects/${projectId}/registries`, { signal }).then(
        (r) => r.items,
      ),
  });

export function createRegistry(projectId: string, input: NewRegistry) {
  return api<Registry>(`/projects/${projectId}/registries`, { method: "POST", body: input });
}

export function deleteRegistry(registryId: string) {
  return api<void>(`/registries/${registryId}`, { method: "DELETE" });
}
