import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";

export type GitProvider = "github" | "gitlab";

export type GitConnection = {
  id: string;
  project_id: string;
  provider: GitProvider;
  display_name: string;
  base_url: string;
  created_at: string;
};

export type Repository = {
  id: string;
  full_name: string;
  default_branch: string;
  private: boolean;
};

export const gitConnectionsQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId, "git-connections"],
    queryFn: ({ signal }) =>
      api<{ items: GitConnection[] }>(`/projects/${projectId}/git-connections`, { signal }).then(
        (r) => r.items,
      ),
  });

export const repositoriesQuery = (connectionId: string) =>
  queryOptions({
    queryKey: ["git-connections", connectionId, "repositories"],
    queryFn: ({ signal }) =>
      api<{ items: Repository[] }>(`/git-connections/${connectionId}/repositories`, {
        signal,
      }).then((r) => r.items),
    staleTime: 60_000,
  });

export function startGitHubInstall(projectId: string) {
  return api<{ install_url: string }>(`/projects/${projectId}/git-connections/github`, {
    method: "POST",
  });
}

export function connectGitLab(projectId: string, baseUrl: string, token: string) {
  return api<GitConnection>(`/projects/${projectId}/git-connections/gitlab`, {
    method: "POST",
    body: { base_url: baseUrl, token },
  });
}

export function deleteGitConnection(connectionId: string) {
  return api<void>(`/git-connections/${connectionId}`, { method: "DELETE" });
}
