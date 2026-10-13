import { Alert, Button, Spinner, toast } from "@heroui/react";
import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import type { BuildResult } from "../../../api/applications";
import { cancelJob } from "../../../api/jobs";
import type { Job } from "../../../api/types";
import { CopyField } from "../../../components/CopyField";
import { JobStatusChip } from "../../../components/JobStatusChip";
import { LogView } from "../../../components/LogView";
import { describeJob, duration } from "../../../lib/describeJob";
import { useHasRole } from "../../../lib/useHasRole";
import { useJobStream } from "../../../lib/useJobStream";

export const Route = createFileRoute("/_app/jobs/$jobId")({
  component: JobPage,
});

function JobPage() {
  const { jobId } = Route.useParams();
  const { job, lines, ended } = useJobStream(jobId);

  if (!job) {
    return <Spinner aria-label="Loading job" />;
  }
  return (
    <div className="flex h-[calc(100dvh-8rem)] flex-col gap-4">
      <JobHeader job={job} />
      {job.type === "build" && <BuildSummary job={job} />}
      {job.type === "deploy_image" && <DeploySummary job={job} />}
      {job.error && (
        <Alert status="danger">
          <Alert.Content>
            <Alert.Title>{job.error.message || "The job failed."}</Alert.Title>
            <Alert.Description>
              Error code <code className="font-mono">{job.error.code}</code>
              {job.attempt_count > 1 && ` after ${job.attempt_count} attempts`}.
            </Alert.Description>
          </Alert.Content>
        </Alert>
      )}
      {lines.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">
          {job.logs_expired
            ? "This job's output was removed to save space; ForgeDeck keeps job output for a limited time."
            : ended
              ? "This job produced no output."
              : "Waiting for output…"}
        </p>
      ) : (
        <LogView lines={lines} />
      )}
    </div>
  );
}

type BuildPayload = {
  application_id: string;
  branch: string;
  commit_sha: string;
  commit_message?: string;
};

/** What a build is building and, once pushed, the exact image to deploy. */
function BuildSummary({ job }: { job: Job }) {
  const payload = job.payload as BuildPayload;
  const result = job.status === "succeeded" ? (job.result as BuildResult | undefined) : undefined;

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">
        <Link
          to="/applications/$applicationId"
          params={{ applicationId: payload.application_id }}
          className="text-[var(--muted)] underline underline-offset-2 hover:text-[var(--foreground)]"
        >
          Application
        </Link>
        <span className="text-[var(--muted)]"> · {payload.branch} · </span>
        <code className="font-mono">{payload.commit_sha.slice(0, 12)}</code>
        {payload.commit_message && <span> {payload.commit_message}</span>}
      </p>
      {result && <CopyField label="image reference" value={result.image_ref} />}
    </div>
  );
}

type DeployPayload = { environment_id: string; commit_sha: string; image_ref: string };

/** What a deployment ships and where, with a way back to its environment. */
function DeploySummary({ job }: { job: Job }) {
  const payload = job.payload as DeployPayload;
  return (
    <p className="text-sm">
      <Link
        to="/environments/$environmentId"
        params={{ environmentId: payload.environment_id }}
        className="text-[var(--muted)] underline underline-offset-2 hover:text-[var(--foreground)]"
      >
        Environment
      </Link>
      <span className="text-[var(--muted)]"> · </span>
      <code className="font-mono">{payload.commit_sha.slice(0, 12)}</code>
      <span className="text-[var(--muted)]"> · </span>
      <code className="break-all font-mono text-xs">{payload.image_ref}</code>
    </p>
  );
}

function JobHeader({ job }: { job: Job }) {
  const canCancel = useHasRole(job.project_id, "developer");
  const cancel = useMutation({
    mutationFn: () => cancelJob(job.id),
    onError: (error) => toast.danger("Could not cancel the job", { description: error.message }),
  });
  const active = job.status === "queued" || job.status === "running";

  return (
    <div>
      <Link to="/jobs" className="text-sm text-[var(--muted)] hover:text-[var(--foreground)]">
        ← Jobs
      </Link>
      <div className="mt-1 flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold tracking-tight">{describeJob(job)}</h1>
        <JobStatusChip status={job.status} />
        {job.status === "running" && <Spinner size="sm" aria-label="Running" />}
        {canCancel && active && (
          <div className="ms-auto">
            <Button
              size="sm"
              variant="danger-soft"
              isPending={cancel.isPending}
              onPress={() => cancel.mutate()}
            >
              Cancel job
            </Button>
          </div>
        )}
      </div>
      <p className="mt-1 text-sm text-[var(--muted)]">
        Created {new Date(job.created_at).toLocaleString()}
        {job.started_at && ` · ran ${duration(job.started_at, job.finished_at)}`}
        {job.attempt_count > 0 && ` · attempt ${job.attempt_count} of ${job.max_attempts}`}
      </p>
    </div>
  );
}
