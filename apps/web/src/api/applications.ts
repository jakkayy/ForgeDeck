import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { JobStatus, List } from "./types";

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
  registry_connection_id: string | null;
  image_repository: string | null;
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
  registry_connection_id?: string;
  image_repository?: string;
};

/** Fields an update may change; image_repository "" removes the image. */
export type ApplicationUpdate = {
  registry_connection_id?: string | null;
  image_repository?: string;
};

export type QueuedBuild = { job_id: string; status: JobStatus; commit_sha: string };

/** What a succeeded build job reports. */
export type BuildResult = {
  commit_sha: string;
  image: string;
  tag: string;
  digest: string;
  image_ref: string;
};

export const applicationsQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId, "applications"],
    queryFn: ({ signal }) =>
      api<List<Application>>(`/projects/${projectId}/applications?limit=100`, { signal }),
  });

export const applicationQuery = (applicationId: string) =>
  queryOptions({
    queryKey: ["applications", applicationId],
    queryFn: ({ signal }) => api<Application>(`/applications/${applicationId}`, { signal }),
  });

export function updateApplication(applicationId: string, input: ApplicationUpdate) {
  return api<Application>(`/applications/${applicationId}`, { method: "PATCH", body: input });
}

/** Builds commitSha, or the head of the application's branch when omitted. */
export function triggerBuild(applicationId: string, commitSha?: string) {
  return api<QueuedBuild>(`/applications/${applicationId}/builds`, {
    method: "POST",
    body: commitSha ? { commit_sha: commitSha } : {},
  });
}

export function createApplication(projectId: string, input: NewApplication) {
  return api<Application>(`/projects/${projectId}/applications`, { method: "POST", body: input });
}

export function deleteApplication(applicationId: string) {
  return api<void>(`/applications/${applicationId}`, { method: "DELETE" });
}
