import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { List } from "./types";

export type Application = {
  id: string;
  project_id: string;
  name: string;
  git_connection_id: string;
  repository_id: string;
  repository_full_name: string;
  branch: string;
  dockerfile_path: string;
  build_context: string;
  test_command: string | null;
  created_at: string;
  updated_at: string;
};

export type NewApplication = {
  name: string;
  git_connection_id: string;
  repository_id: string;
  branch?: string;
  dockerfile_path?: string;
  build_context?: string;
  test_command?: string;
};

export const applicationsQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId, "applications"],
    queryFn: ({ signal }) =>
      api<List<Application>>(`/projects/${projectId}/applications?limit=100`, { signal }),
  });

export function createApplication(projectId: string, input: NewApplication) {
  return api<Application>(`/projects/${projectId}/applications`, { method: "POST", body: input });
}

export function deleteApplication(applicationId: string) {
  return api<void>(`/applications/${applicationId}`, { method: "DELETE" });
}
