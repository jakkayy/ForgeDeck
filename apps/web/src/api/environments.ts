import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { JobStatus, List } from "./types";

export type HealthCheck = {
  type: "http" | "tcp" | "docker";
  path?: string;
  timeout_seconds: number;
  interval_seconds: number;
};

export type ServiceConfig = {
  container_port: number | null;
  published_port: number | null;
  bind_address: "127.0.0.1" | "0.0.0.0";
  env: Record<string, string>;
  /** Names of project secrets the service receives as environment variables. */
  secrets: string[];
  volumes: { name: string; path: string }[];
  restart: "unless-stopped" | "always" | "on-failure" | "no";
  memory_mb: number | null;
  cpus: number | null;
  health_check: HealthCheck;
};

export type Environment = {
  id: string;
  application_id: string;
  name: string;
  server_id: string;
  auto_deploy: boolean;
  config: ServiceConfig;
  created_at: string;
  updated_at: string;
};

/** What the create form sends; the server fills in the rest of the config. */
export type NewEnvironment = {
  name: string;
  server_id: string;
  auto_deploy: boolean;
  config: Partial<ServiceConfig>;
};

export type EnvironmentUpdate = Partial<NewEnvironment>;

export type DeploymentStage = "pull" | "start" | "health_check";

export type Deployment = {
  id: string;
  environment_id: string;
  job_id: string | null;
  source_build_job_id: string | null;
  source_release_id: string | null;
  commit_sha: string;
  image_ref: string;
  status: JobStatus;
  stage: DeploymentStage | null;
  error: { code: string; message: string } | null;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  release_id: string | null;
};

export type Release = {
  id: string;
  application_id: string;
  environment_id: string;
  deployment_id: string;
  commit_sha: string;
  image_ref: string;
  config: ServiceConfig;
  created_at: string;
  current: boolean;
};

export const environmentsQuery = (applicationId: string) =>
  queryOptions({
    queryKey: ["applications", applicationId, "environments"],
    queryFn: ({ signal }) =>
      api<{ items: Environment[] }>(`/applications/${applicationId}/environments`, { signal }).then(
        (r) => r.items,
      ),
  });

export const environmentQuery = (environmentId: string) =>
  queryOptions({
    queryKey: ["environments", environmentId],
    queryFn: ({ signal }) => api<Environment>(`/environments/${environmentId}`, { signal }),
  });

export function createEnvironment(applicationId: string, input: NewEnvironment) {
  return api<Environment>(`/applications/${applicationId}/environments`, {
    method: "POST",
    body: input,
  });
}

export function updateEnvironment(environmentId: string, input: EnvironmentUpdate) {
  return api<Environment>(`/environments/${environmentId}`, { method: "PATCH", body: input });
}

export function deleteEnvironment(environmentId: string) {
  return api<void>(`/environments/${environmentId}`, { method: "DELETE" });
}

/** Deployments poll while one is in progress, so the stage view stays current. */
export const DEPLOYMENT_POLL_MS = 3_000;

export const isDeploymentActive = (d: Pick<Deployment, "status">) =>
  d.status === "queued" || d.status === "running";

export const deploymentsQuery = (environmentId: string, limit = 20) =>
  queryOptions({
    queryKey: ["environments", environmentId, "deployments", { limit }],
    queryFn: ({ signal }) =>
      api<List<Deployment>>(`/environments/${environmentId}/deployments?limit=${limit}`, {
        signal,
      }),
    refetchInterval: (query) =>
      query.state.data?.items.some(isDeploymentActive) ? DEPLOYMENT_POLL_MS : false,
  });

export const releasesQuery = (environmentId: string, limit = 20) =>
  queryOptions({
    queryKey: ["environments", environmentId, "releases", { limit }],
    queryFn: ({ signal }) =>
      api<List<Release>>(`/environments/${environmentId}/releases?limit=${limit}`, { signal }),
  });

export function deployBuild(environmentId: string, buildJobId: string) {
  return api<Deployment>(`/environments/${environmentId}/deployments`, {
    method: "POST",
    body: { build_job_id: buildJobId },
  });
}

export function rollbackRelease(releaseId: string) {
  return api<Deployment>(`/releases/${releaseId}/rollback`, { method: "POST" });
}
