import { Spinner, Tabs, toast } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { projectQuery } from "../../../api/projects";
import { ApplicationsSection } from "../../../components/project/ApplicationsSection";
import { AuditSection } from "../../../components/project/AuditSection";
import { GitSection } from "../../../components/project/GitSection";
import { MembersSection } from "../../../components/project/MembersSection";
import { RegistriesSection } from "../../../components/project/RegistriesSection";
import { SecretsSection } from "../../../components/project/SecretsSection";
import { QueryError } from "../../../components/QueryError";
import { RoleChip } from "../../../components/RoleChip";
import { useHasRole } from "../../../lib/useHasRole";

const tabs = ["applications", "git", "registries", "secrets", "members", "audit"] as const;
type Tab = (typeof tabs)[number];

const gitErrors: Record<string, string> = {
  not_your_installation: "Your GitHub account cannot access that installation.",
  expired: "The connection link expired. Start again from this page.",
  forbidden: "You need the admin role in this project to connect GitHub.",
};

type Search = { tab?: Tab; git?: string; git_error?: string };

export const Route = createFileRoute("/_app/projects/$projectId")({
  validateSearch: (search: Record<string, unknown>): Search => ({
    tab: tabs.includes(search.tab as Tab) ? (search.tab as Tab) : undefined,
    git: typeof search.git === "string" ? search.git : undefined,
    git_error: typeof search.git_error === "string" ? search.git_error : undefined,
  }),
  loader: ({ context, params }) =>
    context.queryClient.prefetchQuery(projectQuery(params.projectId)),
  component: ProjectPage,
});

function ProjectPage() {
  const { projectId } = Route.useParams();
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const project = useQuery(projectQuery(projectId));
  const canSeeAudit = useHasRole(projectId, "admin");
  // The audit log is for admins; anyone else asking for it lands on the default tab.
  const tab =
    search.tab === "audit" && !canSeeAudit ? "applications" : (search.tab ?? "applications");

  // Report the outcome of a GitHub install redirect once, then tidy the URL.
  useEffect(() => {
    if (!search.git && !search.git_error) {
      return;
    }
    if (search.git === "connected") {
      toast.success("GitHub connected");
    } else if (search.git_error) {
      toast.danger("GitHub was not connected", {
        description: gitErrors[search.git_error] ?? "Something went wrong. Try again.",
      });
    }
    navigate({ search: { tab: "git" }, replace: true });
  }, [search.git, search.git_error, navigate]);

  if (project.isPending) {
    return <Spinner aria-label="Loading project" />;
  }
  if (project.isError) {
    return <QueryError error={project.error} onRetry={() => project.refetch()} />;
  }
  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link to="/projects" className="text-sm text-[var(--muted)] hover:text-[var(--foreground)]">
          ← Projects
        </Link>
        <div className="mt-1 flex flex-wrap items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">{project.data.name}</h1>
          <RoleChip role={project.data.role} />
        </div>
      </div>
      {/* isolate keeps the tab indicator's z-index from rising above dialogs */}
      <Tabs
        className="isolate"
        selectedKey={tab}
        onSelectionChange={(key) => navigate({ search: { tab: key as Tab }, replace: true })}
      >
        <Tabs.ListContainer>
          <Tabs.List aria-label="Project sections">
            <Tabs.Tab id="applications">
              Applications
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="git">
              Git
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="registries">
              Registries
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="secrets">
              Secrets
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="members">
              Members
              <Tabs.Indicator />
            </Tabs.Tab>
            {canSeeAudit && (
              <Tabs.Tab id="audit">
                Audit log
                <Tabs.Indicator />
              </Tabs.Tab>
            )}
          </Tabs.List>
        </Tabs.ListContainer>
        <Tabs.Panel id="applications" className="pt-4">
          <ApplicationsSection projectId={projectId} />
        </Tabs.Panel>
        <Tabs.Panel id="git" className="pt-4">
          <GitSection projectId={projectId} />
        </Tabs.Panel>
        <Tabs.Panel id="registries" className="pt-4">
          <RegistriesSection projectId={projectId} />
        </Tabs.Panel>
        <Tabs.Panel id="secrets" className="pt-4">
          <SecretsSection projectId={projectId} />
        </Tabs.Panel>
        <Tabs.Panel id="members" className="pt-4">
          <MembersSection projectId={projectId} />
        </Tabs.Panel>
        {canSeeAudit && (
          <Tabs.Panel id="audit" className="pt-4">
            <AuditSection projectId={projectId} />
          </Tabs.Panel>
        )}
      </Tabs>
    </div>
  );
}
