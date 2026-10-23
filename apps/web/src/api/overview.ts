import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import { listJobs } from "./jobs";
import { SERVER_REFRESH_MS } from "./servers";
import type { JobStatus } from "./types";

export type OverviewDeployment = {
  id: string;
  environment_id: string;
  environment_name: string;
  application_id: string;
  application_name: string;
  project_name: string;
  commit_sha: string;
  status: JobStatus;
  error_code: string | null;
  created_at: string;
  finished_at: string | null;
};

export type Overview = {
  servers: { total: number; online: number; unreachable: number; offline: number; pending: number };
  environments: {
    total: number;
    succeeded: number;
    failed: number;
    in_progress: number;
    not_deployed: number;
  };
  runners: { total: number; online: number };
  recent_deployments: OverviewDeployment[];
  failing_deployments: OverviewDeployment[];
  generated_at: string;
};

export const overviewQuery = queryOptions({
  queryKey: ["overview"],
  queryFn: ({ signal }) => api<Overview>("/overview", { signal }),
  refetchInterval: SERVER_REFRESH_MS,
});

export const recentJobsQuery = queryOptions({
  queryKey: ["overview", "jobs"],
  queryFn: ({ signal }) => listJobs({ limit: 6 }, signal),
  refetchInterval: SERVER_REFRESH_MS,
});
