// Response shapes of the ForgeDeck API (see docs/04-api-spec.md).

export type Role = "owner" | "admin" | "developer" | "viewer";

export type List<T> = { items: T[]; next_cursor: string | null };

export type User = { id: string; email: string };

export type Project = {
  id: string;
  name: string;
  slug: string;
  role: Role;
  created_at: string;
};

export type Member = { user_id: string; email: string; role: Role; added_at: string };

export type ServerStatus = "pending" | "online" | "unreachable" | "offline";

export type Server = {
  id: string;
  project_id: string;
  project_name: string;
  name: string;
  status: ServerStatus;
  last_seen_at: string | null;
  agent_version: string | null;
  docker: { available: boolean; version: string | null };
  resources: { cpu_percent: number; memory_percent: number; disk_percent: number } | null;
  created_at: string;
};

export type ContainerState =
  | "created"
  | "running"
  | "paused"
  | "restarting"
  | "removing"
  | "exited"
  | "dead";

export type Container = {
  container_id: string;
  name: string;
  image: string;
  state: ContainerState;
  status_text: string | null;
  labels: Record<string, string> | null;
};

export type ContainerList = {
  server_id: string;
  observed_at: string | null;
  stale: boolean;
  containers: Container[];
};

export type EnrollmentToken = { token: string; server_name: string; expires_at: string };

export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";

export type Job = {
  id: string;
  project_id: string;
  type: string;
  status: JobStatus;
  target_server_id: string | null;
  payload: unknown;
  result?: unknown;
  error: { code: string; message: string } | null;
  attempt_count: number;
  max_attempts: number;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  logs_expired: boolean;
};

export type LogLine = {
  attempt: number;
  sequence: number;
  stream: "stdout" | "stderr" | "system";
  message: string;
  created_at: string;
};

export type QueuedJob = { job_id: string; status: JobStatus };
