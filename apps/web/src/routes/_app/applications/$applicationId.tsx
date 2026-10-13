import { Alert, Button, Card, Modal, toast } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import {
  type Application,
  applicationQuery,
  type BuildResult,
  triggerBuild,
  updateApplication,
} from "../../../api/applications";
import { isApiError } from "../../../api/client";
import { listJobs } from "../../../api/jobs";
import { registriesQuery } from "../../../api/registries";
import type { Job } from "../../../api/types";
import { ApiForm } from "../../../components/ApiForm";
import { ImageFields, type ImageValue } from "../../../components/application/ImageFields";
import { EnvironmentsSection } from "../../../components/environment/EnvironmentsSection";
import { JobStatusChip } from "../../../components/JobStatusChip";
import { QueryBoundary, StaleNotice, waitingFor } from "../../../components/QueryBoundary";
import { duration } from "../../../lib/describeJob";
import { timeAgo } from "../../../lib/time";
import { useHasRole } from "../../../lib/useHasRole";

const ACTIVE_POLL_MS = 5_000;

export const Route = createFileRoute("/_app/applications/$applicationId")({
  loader: ({ context, params }) =>
    context.queryClient.prefetchQuery(applicationQuery(params.applicationId)),
  component: ApplicationPage,
});

function ApplicationPage() {
  const { applicationId } = Route.useParams();
  const app = useQuery(applicationQuery(applicationId));

  if (app.data === undefined) {
    return waitingFor(app, "application");
  }
  return (
    <div className="flex flex-col gap-6">
      <StaleNotice query={app} label="application" />
      <ApplicationHeader app={app.data} />
      <ImageCard app={app.data} />
      <EnvironmentsSection applicationId={app.data.id} projectId={app.data.project_id} />
      <BuildHistory app={app.data} />
    </div>
  );
}

function ApplicationHeader({ app }: { app: Application }) {
  const canBuild = useHasRole(app.project_id, "developer");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const build = useMutation({
    mutationFn: () => triggerBuild(app.id),
    onSuccess: (queued) => {
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
      toast.success(`Building ${queued.commit_sha.slice(0, 7)}`);
      navigate({ to: "/jobs/$jobId", params: { jobId: queued.job_id } });
    },
    onError: (error) => toast.danger("Could not start the build", { description: error.message }),
  });

  return (
    <div>
      <Link
        to="/projects/$projectId"
        params={{ projectId: app.project_id }}
        search={{ tab: "applications" }}
        className="text-sm text-[var(--muted)] hover:text-[var(--foreground)]"
      >
        ← Project
      </Link>
      <div className="mt-1 flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold tracking-tight">{app.name}</h1>
        {canBuild && (
          <div className="ms-auto">
            <Button
              variant="primary"
              isPending={build.isPending}
              isDisabled={app.image_repository === null}
              onPress={() => build.mutate()}
            >
              Build {app.branch}
            </Button>
          </div>
        )}
      </div>
      <p className="mt-1 font-mono text-sm text-[var(--muted)]">
        {app.repository_full_name} @ {app.branch}
        {app.test_command && ` · tests: ${app.test_command}`}
      </p>
    </div>
  );
}

function ImageCard({ app }: { app: Application }) {
  const registries = useQuery(registriesQuery(app.project_id));
  const canManage = useHasRole(app.project_id, "admin");
  const host = registries.data?.find((r) => r.id === app.registry_connection_id)?.host;

  if (app.image_repository === null) {
    return (
      <Alert status="warning">
        <Alert.Content>
          <Alert.Title>No image destination</Alert.Title>
          <Alert.Description>
            Builds need a registry and image to push to.
            {canManage ? " Choose them to start building." : " Ask a project admin to set them."}
          </Alert.Description>
        </Alert.Content>
        {canManage && <EditImageDialog app={app} />}
      </Alert>
    );
  }
  return (
    <Card className="flex flex-row flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
      <div className="min-w-48 flex-1">
        <p className="text-xs text-[var(--muted)]">Image</p>
        <p className="font-mono text-sm">
          {host ?? "…"}/{app.image_repository}
        </p>
      </div>
      {canManage && <EditImageDialog app={app} />}
    </Card>
  );
}

function EditImageDialog({ app }: { app: Application }) {
  const queryClient = useQueryClient();
  const [image, setImage] = useState<ImageValue>({
    registryId: app.registry_connection_id,
    repository: app.image_repository ?? "",
  });
  const save = useMutation({
    mutationFn: () =>
      updateApplication(app.id, {
        registry_connection_id: image.registryId,
        image_repository: image.registryId ? image.repository.trim() : "",
      }),
    onSuccess: (updated) => {
      queryClient.setQueryData(applicationQuery(app.id).queryKey, updated);
      queryClient.invalidateQueries({ queryKey: ["projects", app.project_id, "applications"] });
      toast.success("Image destination saved");
    },
  });

  return (
    <Modal onOpenChange={(open) => !open && save.reset()}>
      <Button size="sm" variant="secondary">
        {app.image_repository === null ? "Choose image" : "Change"}
      </Button>
      <Modal.Backdrop>
        <Modal.Container size="sm">
          <Modal.Dialog>
            {({ close }) => {
              const onSubmit = (event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                save.mutate(undefined, { onSuccess: close });
              };
              return (
                <ApiForm onSubmit={onSubmit} error={save.error} onClearError={save.reset}>
                  <Modal.Header>
                    <Modal.Heading>Image destination</Modal.Heading>
                  </Modal.Header>
                  <Modal.Body className="flex flex-col gap-4">
                    <ImageFields projectId={app.project_id} value={image} onChange={setImage} />
                    {save.error && !isApiError(save.error, 422) && (
                      <Alert status="danger">
                        <Alert.Content>
                          <Alert.Description>{save.error.message}</Alert.Description>
                        </Alert.Content>
                      </Alert>
                    )}
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary" onPress={close}>
                      Cancel
                    </Button>
                    <Button type="submit" variant="primary" isPending={save.isPending}>
                      Save
                    </Button>
                  </Modal.Footer>
                </ApiForm>
              );
            }}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}

const isActive = (job: Job) => job.status === "queued" || job.status === "running";

function BuildHistory({ app }: { app: Application }) {
  const builds = useQuery({
    queryKey: ["jobs", { application_id: app.id, type: "build" }],
    queryFn: ({ signal }) => listJobs({ application_id: app.id, type: "build", limit: 20 }, signal),
    // Refresh only while a build is still in progress.
    refetchInterval: (query) => (query.state.data?.items.some(isActive) ? ACTIVE_POLL_MS : false),
  });

  return (
    <section aria-label="Builds" className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Recent builds</h2>
      <QueryBoundary
        query={builds}
        label="builds"
        isEmpty={(data) => data.items.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            No builds yet. Push to {app.branch} or start one with the Build button.
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.items.map((job) => (
                <BuildRow key={job.id} job={job} />
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </section>
  );
}

function BuildRow({ job }: { job: Job }) {
  const payload = (job.payload ?? {}) as { commit_sha?: string; commit_message?: string };
  const result = job.status === "succeeded" ? (job.result as BuildResult | undefined) : undefined;

  return (
    <li className="flex flex-wrap items-center gap-x-6 gap-y-1 px-4 py-3">
      <Link
        to="/jobs/$jobId"
        params={{ jobId: job.id }}
        className="flex min-w-56 flex-1 items-baseline gap-2 hover:underline"
      >
        <code className="font-mono text-sm">{payload.commit_sha?.slice(0, 7)}</code>
        <span className="truncate text-sm">{payload.commit_message || "Build"}</span>
      </Link>
      <JobStatusChip status={job.status} />
      <p className="w-36 truncate font-mono text-xs text-[var(--muted)]">
        {result ? result.digest.slice(0, 19) : (job.error?.code ?? "")}
      </p>
      <p className="w-28 text-sm text-[var(--muted)]">{timeAgo(job.created_at)}</p>
      <p className="w-16 text-end text-sm tabular-nums text-[var(--muted)]">
        {duration(job.started_at, job.finished_at)}
      </p>
    </li>
  );
}
