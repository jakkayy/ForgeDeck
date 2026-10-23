import { AlertDialog, Button, Card, toast } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { meQuery } from "../../../api/auth";
import { deleteRunner, type Runner, runnersQuery } from "../../../api/runners";
import { AddRunnerDialog } from "../../../components/AddRunnerDialog";
import { EmptyState } from "../../../components/EmptyState";
import { PageHeader } from "../../../components/PageHeader";
import { QueryBoundary } from "../../../components/QueryBoundary";
import { RunnerStatusChip } from "../../../components/RunnerStatusChip";
import { timeAgo } from "../../../lib/time";

export const Route = createFileRoute("/_app/runners/")({
  component: RunnersPage,
});

function RunnersPage() {
  const me = useQuery(meQuery);
  if (me.data && !me.data.is_admin) {
    return (
      <>
        <PageHeader title="Runners" />
        <EmptyState
          title="Only an administrator can manage Runners"
          description="Runners build the images of every project, so they belong to the whole installation. Ask the administrator of this ForgeDeck to add one."
        />
      </>
    );
  }
  return <AdminRunners />;
}

function AdminRunners() {
  const runners = useQuery(runnersQuery);
  return (
    <>
      <PageHeader
        title="Runners"
        description="Build hosts that turn your repositories into images."
        actions={<AddRunnerDialog />}
      />
      <QueryBoundary
        query={runners}
        label="runners"
        isEmpty={(data) => data.length === 0}
        empty={
          <EmptyState
            title="No Runners yet"
            description="A Runner builds your images. Add one, then install it on a dedicated build host with the command shown."
          />
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.map((runner) => (
                <RunnerRow key={runner.id} runner={runner} />
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </>
  );
}

function RunnerRow({ runner }: { runner: Runner }) {
  return (
    <li className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
      <div className="min-w-48 flex-1">
        <p className="font-medium">{runner.name}</p>
        <p className="text-xs text-[var(--muted)]">
          {runner.version ? `Version ${runner.version}` : "Version not reported yet"}
        </p>
      </div>
      <RunnerStatusChip status={runner.status} />
      <p className="w-36 text-sm text-[var(--muted)]">Seen {timeAgo(runner.last_seen_at)}</p>
      <RemoveRunner runner={runner} />
    </li>
  );
}

function RemoveRunner({ runner }: { runner: Runner }) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteRunner(runner.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: runnersQuery.queryKey });
      toast.success(`${runner.name} was removed`);
    },
    onError: (error) =>
      toast.danger(`Could not remove ${runner.name}`, { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="danger-soft" isPending={remove.isPending}>
        Remove
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Remove {runner.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body className="flex flex-col gap-2 text-sm">
                  <p>Its credential is revoked immediately and it stops receiving builds.</p>
                  <p className="text-[var(--muted)]">
                    A build it is running fails and is retried by another Runner. The host keeps the
                    software until you run{" "}
                    <code className="font-mono">install-runner.sh --uninstall</code> there.
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
                    Remove Runner
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
