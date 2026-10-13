import { api } from "./client";
import type { List } from "./types";

export type AuditEntry = {
  id: string;
  actor: string;
  action: string;
  target_type: string;
  target_id: string | null;
  target_name: string | null;
  metadata: Record<string, unknown>;
  request_id: string | null;
  created_at: string;
};

export function listAudit(
  projectId: string,
  params: { action?: string; cursor?: string },
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({ limit: "50" });
  if (params.action) {
    query.set("action", params.action);
  }
  if (params.cursor) {
    query.set("cursor", params.cursor);
  }
  return api<List<AuditEntry>>(`/projects/${projectId}/audit-logs?${query}`, { signal });
}
