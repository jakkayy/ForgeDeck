import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { List, Member, Project, Role } from "./types";

export const projectsQuery = queryOptions({
  queryKey: ["projects"],
  queryFn: ({ signal }) => api<List<Project>>("/projects?limit=100", { signal }),
});

export function createProject(input: { name: string; slug?: string }) {
  return api<Project>("/projects", { method: "POST", body: input });
}

export const projectQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId],
    queryFn: ({ signal }) => api<Project>(`/projects/${projectId}`, { signal }),
  });

export const membersQuery = (projectId: string) =>
  queryOptions({
    queryKey: ["projects", projectId, "members"],
    queryFn: ({ signal }) =>
      api<List<Member>>(`/projects/${projectId}/members?limit=100`, { signal }),
  });

export function addMember(projectId: string, email: string, role: Role) {
  return api<Member>(`/projects/${projectId}/members`, { method: "POST", body: { email, role } });
}

export function removeMember(projectId: string, userId: string) {
  return api<void>(`/projects/${projectId}/members/${userId}`, { method: "DELETE" });
}
