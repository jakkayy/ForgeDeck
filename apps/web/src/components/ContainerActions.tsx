import { AlertDialog, Button, toast } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { waitForJob } from "../api/jobs";
import { type ContainerAction, containersQuery, requestContainerAction } from "../api/servers";
import type { Container } from "../api/types";

const progress: Record<ContainerAction, string> = {
  start: "Starting",
  stop: "Stopping",
  restart: "Restarting",
};

type Props = { serverId: string; container: Container };

/**
 * Start/Stop/Restart for one container. Stop and Restart interrupt service,
 * so they ask for confirmation. Success is reported only after the Agent
 * finishes the job, never when the request is merely accepted.
 */
export function ContainerActions({ serverId, container }: Props) {
  const queryClient = useQueryClient();
  const run = useMutation({
    mutationFn: async (action: ContainerAction) => {
      const toastId = toast(`${progress[action]} ${container.name}…`, {
        isLoading: true,
        timeout: 0,
      });
      try {
        const queued = await requestContainerAction(serverId, container.container_id, action);
        const job = await waitForJob(queued.job_id);
        if (job.status !== "succeeded") {
          throw new Error(job.error?.message ?? `The ${action} did not complete.`);
        }
        toast.update(toastId, `${container.name}: ${action} done`, {
          variant: "success",
          timeout: 4000,
        });
      } catch (error) {
        toast.update(toastId, `Could not ${action} ${container.name}`, {
          variant: "danger",
          description: error instanceof Error ? error.message : String(error),
          timeout: 8000,
        });
        throw error;
      }
    },
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: containersQuery(serverId).queryKey }),
  });

  const isRunning = container.state === "running" || container.state === "restarting";
  return (
    <div className="flex w-40 justify-end gap-2">
      {isRunning ? (
        <>
          <ConfirmAction
            action="restart"
            container={container}
            isPending={run.isPending}
            onConfirm={() => run.mutate("restart")}
          />
          <ConfirmAction
            action="stop"
            container={container}
            isPending={run.isPending}
            onConfirm={() => run.mutate("stop")}
          />
        </>
      ) : (
        <Button
          size="sm"
          variant="secondary"
          isPending={run.isPending}
          onPress={() => run.mutate("start")}
        >
          Start
        </Button>
      )}
    </div>
  );
}

const confirmCopy = {
  restart: {
    label: "Restart",
    body: "The container stops and starts again. Requests during the restart may fail.",
  },
  stop: {
    label: "Stop",
    body: "The container stops serving until you start it again.",
  },
};

type ConfirmProps = {
  action: "restart" | "stop";
  container: Container;
  isPending: boolean;
  onConfirm: () => void;
};

function ConfirmAction({ action, container, isPending, onConfirm }: ConfirmProps) {
  const copy = confirmCopy[action];
  return (
    <AlertDialog>
      <Button
        size="sm"
        variant={action === "stop" ? "danger-soft" : "secondary"}
        isDisabled={isPending}
      >
        {copy.label}
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>
                    {copy.label} {container.name}?
                  </AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">{copy.body}</p>
                </AlertDialog.Body>
                <AlertDialog.Footer>
                  <Button variant="secondary" onPress={close}>
                    Cancel
                  </Button>
                  <Button
                    variant={action === "stop" ? "danger" : "primary"}
                    onPress={() => {
                      onConfirm();
                      close();
                    }}
                  >
                    {copy.label}
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
