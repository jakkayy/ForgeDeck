import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { List, Project } from "./types";

export const projectsQuery = queryOptions({
  queryKey: ["projects"],
  queryFn: ({ signal }) => api<List<Project>>("/projects?limit=100", { signal }),
});

export function createProject(input: { name: string; slug?: string }) {
  return api<Project>("/projects", { method: "POST", body: input });
}
