import { AlertDialog, Button, Card, Chip, Spinner, toast } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type Deployment,
  deleteEnvironment,
  deployBuild,
  deploymentsQuery,
  type Environment,
  environmentsQuery,
  releasesQuery,
} from "../../api/environments";
import { listJobs } from "../../api/jobs";
import { serversQuery } from "../../api/servers";
import { shortImage } from "../../lib/serviceConfig";
import { timeAgo } from "../../lib/time";
import { useHasRole } from "../../lib/useHasRole";
import { JobStatusChip } from "../JobStatusChip";
import { QueryError } from "../QueryError";
import { EnvironmentDialog } from "./EnvironmentDialog";

type Props = { applicationId: string; projectId: string };

/** The application's environments: what each runs now and the means to change it. */
export function EnvironmentsSection({ applicationId, projectId }: Props) {
  const environments = useQuery(environmentsQuery(applicationId));
  const canManage = useHasRole(projectId, "admin");
  // The newest successful build is what "Deploy" ships.
  const latestBuild = useQuery({
    queryKey: [
      "jobs",
      { application_id: applicationId, type: "build", status: "succeeded", limit: 1 },
    ],
    queryFn: ({ signal }) =>
      listJobs(
        { application_id: applicationId, type: "build", status: "succeeded", limit: 1 },
        signal,
      ),
  });

  return (
    <section aria-label="Environments" className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold">Environments</h2>
        {canManage && <EnvironmentDialog applicationId={applicationId} projectId={projectId} />}
      </div>
      {environments.isPending && <Spinner aria-label="Loading environments" />}
      {environments.isError && (
        <QueryError error={environments.error} onRetry={() => environments.refetch()} />
      )}
      {environments.isSuccess && environments.data.length === 0 && (
        <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
          No environments yet. An environment is a server and how the service runs on it, such as
          production.
        </Card>
      )}
      {environments.isSuccess && environments.data.length > 0 && (
        <Card className="p-0">
          <ul className="divide-y divide-[var(--separator)]">
            {environments.data.map((environment) => (
              <EnvironmentRow
                key={environment.id}
                environment={environment}
                applicationId={applicationId}
                projectId={projectId}
                latestBuild={latestBuild.data?.items[0]}
              />
            ))}
          </ul>
        </Card>
      )}
    </section>
  );
}

type RowProps = {
  environment: Environment;
  applicationId: string;
  projectId: string;
  latestBuild: { id: string; payload: unknown } | undefined;
};

function EnvironmentRow({ environment, applicationId, projectId, latestBuild }: RowProps) {
  const canManage = useHasRole(projectId, "admin");
  const canDeploy = useHasRole(projectId, "developer");
  const servers = useQuery(serversQuery);
  const serverName = servers.data?.items.find((s) => s.id === environment.server_id)?.name;
  const release = useQuery(releasesQuery(environment.id, 1));
  const deployments = useQuery(deploymentsQuery(environment.id, 1));
  const current = release.data?.items[0];
  const last: Deployment | undefined = deployments.data?.items[0];
  const { published_port, container_port, bind_address } = environment.config;

  return (
    <li className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
      <div className="min-w-48 flex-1">
        <p className="flex items-center gap-2 font-medium">
          {environment.name}
          {environment.auto_deploy && (
            <Chip size="sm" variant="soft">
              auto-deploy
            </Chip>
          )}
        </p>
        <p className="text-xs text-[var(--muted)]">
          {serverName ?? "server"}
          {published_port !== null && ` · ${bind_address}:${published_port} → ${container_port}`}
        </p>
      </div>
      <div className="min-w-48 flex-1 text-sm">
        {current ? (
          <>
            <p className="font-mono text-xs">
              <code>{current.commit_sha.slice(0, 7)}</code> · {shortImage(current.image_ref)}
            </p>
            <p className="text-xs text-[var(--muted)]">
              Running since {timeAgo(current.created_at)}
            </p>
          </>
        ) : (
          <p className="text-[var(--muted)]">Nothing deployed yet</p>
        )}
      </div>
      {last && (
        <div className="flex items-center gap-2">
          <JobStatusChip status={last.status} />
          <p className="text-xs text-[var(--muted)]">{timeAgo(last.created_at)}</p>
        </div>
      )}
      <div className="flex gap-2">
        {canDeploy && (
          <DeployButton
            environment={environment}
            applicationId={applicationId}
            build={latestBuild}
          />
        )}
        {canManage && (
          <>
            <EnvironmentDialog
              applicationId={applicationId}
              projectId={projectId}
              environment={environment}
            />
            <DeleteEnvironment environment={environment} applicationId={applicationId} />
          </>
        )}
      </div>
    </li>
  );
}

function DeployButton({
  environment,
  applicationId,
  build,
}: {
  environment: Environment;
  applicationId: string;
  build: { id: string; payload: unknown } | undefined;
}) {
  const queryClient = useQueryClient();
  const deploy = useMutation({
    mutationFn: () => deployBuild(environment.id, build?.id ?? ""),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["environments", environment.id] });
      queryClient.invalidateQueries({ queryKey: environmentsQuery(applicationId).queryKey });
      toast.success(`Deploying to ${environment.name}`);
    },
    onError: (error) =>
      toast.danger("Could not start the deployment", { description: error.message }),
  });
  const commit = (build?.payload as { commit_sha?: string } | undefined)?.commit_sha;

  return (
    <AlertDialog>
      <Button size="sm" variant="primary" isDisabled={!build} isPending={deploy.isPending}>
        Deploy
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Deploy to {environment.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    The newest successful build{commit ? ` (${commit.slice(0, 7)})` : ""} replaces
                    what runs in {environment.name}. The service restarts, so there is a short gap.
                    If the new version does not become healthy, the previous one is restored.
                  </p>
                </AlertDialog.Body>
                <AlertDialog.Footer>
                  <Button variant="secondary" onPress={close}>
                    Cancel
                  </Button>
                  <Button
                    variant="primary"
                    onPress={() => {
                      deploy.mutate();
                      close();
                    }}
                  >
                    Deploy
                  </Button>
                </AlertDialog.Footer>
              </>
            )}
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  );
}

function DeleteEnvironment({
  environment,
  applicationId,
}: {
  environment: Environment;
  applicationId: string;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteEnvironment(environment.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: environmentsQuery(applicationId).queryKey });
      toast.success(`${environment.name} was deleted`);
    },
    onError: (error) =>
      toast.danger(`Could not delete ${environment.name}`, { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="danger-soft" isPending={remove.isPending}>
        Delete
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Delete {environment.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    Its deployment and release history is deleted. The service keeps running on the
                    server and its volumes stay; ForgeDeck just stops managing it.
                  </p>
                </AlertDialog.Body>
                <AlertDialog.Footer>
                  <Button variant="secondary" onPress={close}>
                    Cancel
                  </Button>
                  <Button
                    variant="danger"
                    onPress={() => {
                      remove.mutate();
                      close();
                    }}
                  >
                    Delete environment
                  </Button>
                </AlertDialog.Footer>
              </>
            )}
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  );
}
