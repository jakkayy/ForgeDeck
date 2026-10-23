import { Card } from "@heroui/react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { applicationsQuery } from "../../api/applications";
import { environmentsQuery, releasesQuery } from "../../api/environments";
import { gitConnectionsQuery } from "../../api/git";
import { overviewQuery } from "../../api/overview";
import { registriesQuery } from "../../api/registries";
import { serversQuery } from "../../api/servers";
import { useHasRole } from "../../lib/useHasRole";

const INSPECTED_APPLICATIONS = 5;

type StepStatus = "done" | "next" | "todo";

type Step = {
  title: string;
  detail: string;
  done: boolean;
  action: { label: string; to: Destination };
};

type Destination =
  | { kind: "tab"; tab: "git" | "registries" | "applications" }
  | { kind: "servers" }
  | { kind: "runners" }
  | { kind: "application"; id: string };

/** Checklist from an empty project to a healthy deployment; hidden once a release runs or for non-admins. */
export function SetupGuide({ projectId }: { projectId: string }) {
  const canSetUp = useHasRole(projectId, "admin");
  const git = useQuery(gitConnectionsQuery(projectId));
  const registries = useQuery(registriesQuery(projectId));
  const servers = useQuery(serversQuery);
  const overview = useQuery(overviewQuery);
  const applications = useQuery(applicationsQuery(projectId));

  const apps = (applications.data?.items ?? []).slice(0, INSPECTED_APPLICATIONS);
  const environments = useQueries({ queries: apps.map((app) => environmentsQuery(app.id)) });
  const environmentIds = environments.flatMap((q) => q.data ?? []).map((e) => e.id);
  const releases = useQueries({ queries: environmentIds.map((id) => releasesQuery(id, 1)) });

  const loading = [git, registries, servers, overview, applications].some((q) => q.isPending);
  if (!canSetUp || loading) {
    return null;
  }

  const projectServers = (servers.data?.items ?? []).filter((s) => s.project_id === projectId);
  const deployed = releases.some((q) => (q.data?.items.length ?? 0) > 0);
  const firstApp = apps[0];
  const steps: Step[] = [
    {
      title: "Connect GitHub or GitLab",
      detail: "So ForgeDeck can read your repositories and hear about pushes.",
      done: (git.data?.length ?? 0) > 0,
      action: { label: "Connect", to: { kind: "tab", tab: "git" } },
    },
    {
      title: "Add a container registry",
      detail: "Where built images are pushed, such as ghcr.io or Docker Hub.",
      done: (registries.data?.length ?? 0) > 0,
      action: { label: "Add registry", to: { kind: "tab", tab: "registries" } },
    },
    {
      title: "Add a server and install its Agent",
      detail: "A Docker host ForgeDeck deploys to. The Agent connects outward; no inbound port.",
      done: projectServers.some((s) => s.status === "online"),
      action: {
        label: projectServers.length > 0 ? "Check the Agent" : "Add server",
        to: { kind: "servers" },
      },
    },
    {
      title: "Add a Runner",
      detail:
        "The build host that turns your repository into an image. Without one, a build waits forever.",
      done: (overview.data?.runners.online ?? 0) > 0,
      action: {
        label: (overview.data?.runners.total ?? 0) > 0 ? "Check the Runner" : "Add Runner",
        to: { kind: "runners" },
      },
    },
    {
      title: "Create an application",
      detail: "One repository and branch, built into an image on every push.",
      done: apps.some((app) => app.image_repository !== null),
      action: { label: "New application", to: { kind: "tab", tab: "applications" } },
    },
    {
      title: "Add an environment",
      detail: "A server and how the service runs on it: ports, health check, variables.",
      done: environmentIds.length > 0,
      action: firstApp
        ? { label: "Add environment", to: { kind: "application", id: firstApp.id } }
        : { label: "New application", to: { kind: "tab", tab: "applications" } },
    },
    {
      title: "Build and deploy",
      detail:
        "Push to the branch (or press Build), then Deploy. A release is recorded when it is healthy.",
      done: deployed,
      action: firstApp
        ? { label: "Open the application", to: { kind: "application", id: firstApp.id } }
        : { label: "New application", to: { kind: "tab", tab: "applications" } },
    },
  ];
  if (steps.every((s) => s.done)) {
    return null;
  }
  const next = steps.findIndex((s) => !s.done);
  const statusOf = (i: number): StepStatus =>
    steps[i]?.done ? "done" : i === next ? "next" : "todo";

  return (
    <Card className="flex flex-col gap-3 p-4" aria-label="Getting started">
      <div>
        <h2 className="text-base font-semibold">Get this project running</h2>
        <p className="text-sm text-[var(--muted)]">
          {steps.filter((s) => s.done).length} of {steps.length} steps done.
        </p>
      </div>
      <ol className="flex flex-col gap-2">
        {steps.map((step, i) => (
          <li key={step.title} className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <StepMarker status={statusOf(i)} number={i + 1} />
            <div className="min-w-48 flex-1">
              <p
                className={
                  statusOf(i) === "done"
                    ? "text-sm text-[var(--muted)] line-through"
                    : "text-sm font-medium"
                }
              >
                {step.title}
              </p>
              {statusOf(i) !== "done" && (
                <p className="text-xs text-[var(--muted)]">{step.detail}</p>
              )}
            </div>
            {statusOf(i) !== "done" && (
              <StepLink projectId={projectId} step={step} primary={statusOf(i) === "next"} />
            )}
          </li>
        ))}
      </ol>
    </Card>
  );
}

function StepMarker({ status, number }: { status: StepStatus; number: number }) {
  const style = {
    done: "bg-[var(--success)] text-[var(--success-foreground)]",
    next: "bg-[var(--accent)] text-[var(--accent-foreground)]",
    todo: "bg-[var(--default)] text-[var(--muted)]",
  }[status];
  return (
    <span
      className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs ${style}`}
    >
      <span aria-hidden="true">{status === "done" ? "✓" : number}</span>
      <span className="sr-only">
        {status === "done" ? "Done" : status === "next" ? "Next step" : "Not done yet"}
      </span>
    </span>
  );
}

const linkClass = (primary: boolean) =>
  `rounded-full px-3 py-1 text-sm ${
    primary
      ? "bg-[var(--accent)] text-[var(--accent-foreground)]"
      : "bg-[var(--default)] text-[var(--foreground)]"
  }`;

function StepLink({
  projectId,
  step,
  primary,
}: {
  projectId: string;
  step: Step;
  primary: boolean;
}) {
  const { to, label } = step.action;
  if (to.kind === "tab") {
    return (
      <Link
        to="/projects/$projectId"
        params={{ projectId }}
        search={{ tab: to.tab }}
        className={linkClass(primary)}
      >
        {label}
      </Link>
    );
  }
  if (to.kind === "servers") {
    return (
      <Link to="/servers" className={linkClass(primary)}>
        {label}
      </Link>
    );
  }
  if (to.kind === "runners") {
    return (
      <Link to="/runners" className={linkClass(primary)}>
        {label}
      </Link>
    );
  }
  return (
    <Link
      to="/applications/$applicationId"
      params={{ applicationId: to.id }}
      className={linkClass(primary)}
    >
      {label}
    </Link>
  );
}
