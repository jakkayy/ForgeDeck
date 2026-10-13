import { Alert, Button, Spinner, toast } from "@heroui/react";
import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { cancelJob } from "../../../api/jobs";
import type { Job } from "../../../api/types";
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
          {ended ? "This job produced no output." : "Waiting for output…"}
        </p>
      ) : (
        <LogView lines={lines} />
      )}
    </div>
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
