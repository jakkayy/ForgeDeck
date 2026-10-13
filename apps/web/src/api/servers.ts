import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import { pagedQuery } from "./paged";
import type { ContainerList, EnrollmentToken, List, QueuedJob, Server } from "./types";

/** Matches the Agent heartbeat interval, so status is never more than one beat behind. */
export const SERVER_REFRESH_MS = 20_000;

export const serversQuery = queryOptions({
  queryKey: ["servers"],
  queryFn: ({ signal }) => api<List<Server>>("/servers?limit=100", { signal }),
  refetchInterval: SERVER_REFRESH_MS,
});

export const serverListQuery = (filter: { q?: string; status?: string }) =>
  pagedQuery<Server>({
    key: ["servers"],
    path: "/servers",
    params: filter,
    refetchInterval: SERVER_REFRESH_MS,
  });

export function createEnrollmentToken(projectId: string, serverName: string) {
  return api<EnrollmentToken>(`/projects/${projectId}/servers/enrollment-tokens`, {
    method: "POST",
    body: { server_name: serverName },
  });
}

export const serverQuery = (serverId: string) =>
  queryOptions({
    queryKey: ["servers", serverId],
    queryFn: ({ signal }) => api<Server>(`/servers/${serverId}`, { signal }),
    refetchInterval: SERVER_REFRESH_MS,
  });

export const containersQuery = (serverId: string) =>
  queryOptions({
    queryKey: ["servers", serverId, "containers"],
    queryFn: ({ signal }) => api<ContainerList>(`/servers/${serverId}/containers`, { signal }),
    refetchInterval: SERVER_REFRESH_MS,
  });

export type ContainerAction = "start" | "stop" | "restart";

export function requestContainerAction(
  serverId: string,
  containerId: string,
  action: ContainerAction,
) {
  return api<QueuedJob>(`/servers/${serverId}/containers/${containerId}/actions`, {
    method: "POST",
    body: { action },
  });
}

export function requestContainerLogs(serverId: string, containerId: string, tailLines: number) {
  return api<QueuedJob>(`/servers/${serverId}/containers/${containerId}/logs`, {
    method: "POST",
    body: { tail_lines: tailLines, timestamps: true },
  });
}

/** Asks the server's Agent to replace its credential at its next heartbeat. */
export function requestCredentialRotation(serverId: string) {
  return api<void>(`/servers/${serverId}/credential-rotation`, { method: "POST" });
}

export function deleteServer(serverId: string) {
  return api<void>(`/servers/${serverId}`, { method: "DELETE" });
}
