import type { AuditEntry } from "../api/audit";

/** What each audited action says, as a verb phrase after the actor's name. */
const phrases: Record<string, (e: AuditEntry) => string> = {
  "project.create": () => "created the project",
  "project.update": () => "changed the project",
  "project.delete": () => "deleted the project",
  "member.add": (e) =>
    `added ${e.target_name ?? "a member"} as ${String(e.metadata.role ?? "a member")}`,
  "member.remove": () => "removed a member",
  "secret.set": (e) =>
    `${e.metadata.created ? "added" : "replaced"} the secret ${e.target_name ?? ""}`,
  "secret.delete": (e) => `deleted the secret ${e.target_name ?? ""}`,
  "registry.create": (e) => `added the registry ${e.target_name ?? ""}`,
  "registry.delete": (e) => `removed the registry ${e.target_name ?? ""}`,
  "git_connection.create": (e) => `connected ${e.target_name ?? "a Git provider"}`,
  "git_connection.install_start": () => "started connecting GitHub",
  "git_connection.delete": (e) => `disconnected ${e.target_name ?? "a Git provider"}`,
  "application.create": (e) => `created the application ${e.target_name ?? ""}`,
  "application.update": (e) => `changed the application ${e.target_name ?? ""}`,
  "application.delete": (e) => `deleted the application ${e.target_name ?? ""}`,
  "build.trigger": (e) => `started a build of ${e.target_name ?? "an application"}`,
  "build.queue": (e) => `queued a build of ${e.target_name ?? "an application"} for a push`,
  "environment.create": (e) => `created the environment ${e.target_name ?? ""}`,
  "environment.update": (e) => `changed the environment ${e.target_name ?? ""}`,
  "environment.delete": (e) => `deleted the environment ${e.target_name ?? ""}`,
  "deployment.start": (e) =>
    e.metadata.trigger === "auto_deploy"
      ? `deployed automatically to ${e.target_name ?? "an environment"}`
      : `started a deployment to ${e.target_name ?? "an environment"}`,
  "release.rollback": (e) => `rolled ${e.target_name ?? "an environment"} back`,
  "job.cancel": (e) => `cancelled a ${e.target_name ?? ""} job`,
  "server.enroll": (e) => `enrolled as ${e.target_name ?? "a server"}`,
  "server.delete": (e) => `removed the server ${e.target_name ?? ""}`,
  "enrollment_token.create": (e) =>
    `created an enrollment token for ${e.target_name ?? "a server"}`,
  "container.action": (e) =>
    `asked to ${String(e.metadata.action ?? "act on")} a container (${(e.target_id ?? "").slice(0, 12)})`,
  "container.logs_fetch": (e) =>
    `fetched logs of a container (${(e.target_id ?? "").slice(0, 12)})`,
  "user.login": () => "signed in",
};

export function describeAudit(entry: AuditEntry): string {
  return (phrases[entry.action]?.(entry) ?? `${entry.action} ${entry.target_name ?? ""}`).trim();
}

/** Actions offered in the filter, grouped as people think of them. */
export const auditActions = Object.keys(phrases).sort();
