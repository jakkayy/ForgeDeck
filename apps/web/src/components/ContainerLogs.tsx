import { Alert, Button, Drawer, ListBox, Select, Spinner } from "@heroui/react";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { getAllJobLogs, waitForJob } from "../api/jobs";
import { requestContainerLogs } from "../api/servers";
import type { Container, LogLine } from "../api/types";
import { LogView } from "./LogView";

const tailOptions = [100, 500, 2000];

type FetchedLogs = { lines: LogLine[]; truncated: boolean; fetchedAt: Date };

async function fetchLogs(
  serverId: string,
  container: Container,
  tail: number,
): Promise<FetchedLogs> {
  const queued = await requestContainerLogs(serverId, container.container_id, tail);
  const job = await waitForJob(queued.job_id);
  if (job.status !== "succeeded") {
    throw new Error(job.error?.message ?? "The Agent could not read the logs.");
  }
  const lines = await getAllJobLogs(job.id);
  const truncated = (job.result as { truncated?: boolean } | undefined)?.truncated === true;
  return { lines, truncated, fetchedAt: new Date() };
}

type Props = { serverId: string; container: Container; isDisabled?: boolean };

/** Past output of one container, rendered as text (never HTML); redaction happens server-side. */
export function ContainerLogs({ serverId, container, isDisabled = false }: Props) {
  const [tail, setTail] = useState(500);
  const logs = useMutation({
    mutationFn: (lines: number) => fetchLogs(serverId, container, lines),
  });

  return (
    <Drawer onOpenChange={(open) => open && logs.mutate(tail)}>
      <Button size="sm" variant="tertiary" isDisabled={isDisabled}>
        Logs
      </Button>
      <Drawer.Backdrop>
        <Drawer.Content placement="right">
          <Drawer.Dialog className="w-full max-w-[95vw] sm:w-[52rem]">
            <Drawer.Header className="flex-row flex-wrap items-center gap-3 pe-10">
              <Drawer.Heading className="flex-1">Logs · {container.name}</Drawer.Heading>
              <Select
                aria-label="Lines to fetch"
                className="w-36"
                selectedKey={String(tail)}
                onSelectionChange={(key) => setTail(Number(key))}
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {tailOptions.map((n) => (
                      <ListBox.Item key={n} id={String(n)} textValue={`Last ${n} lines`}>
                        Last {n} lines
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
              <Button
                size="sm"
                variant="secondary"
                isPending={logs.isPending}
                onPress={() => logs.mutate(tail)}
              >
                Refresh
              </Button>
              <Drawer.CloseTrigger />
            </Drawer.Header>
            <Drawer.Body className="flex min-h-0 flex-1 flex-col gap-3">
              {logs.isPending && (
                <p className="flex items-center gap-2 text-sm text-[var(--muted)]">
                  <Spinner size="sm" aria-hidden="true" /> Asking the Agent for logs…
                </p>
              )}
              {logs.isError && (
                <Alert status="danger">
                  <Alert.Content>
                    <Alert.Description>{logs.error.message}</Alert.Description>
                  </Alert.Content>
                </Alert>
              )}
              {logs.isSuccess && <LogOutput logs={logs.data} />}
            </Drawer.Body>
          </Drawer.Dialog>
        </Drawer.Content>
      </Drawer.Backdrop>
    </Drawer>
  );
}

function LogOutput({ logs }: { logs: FetchedLogs }) {
  if (logs.lines.length === 0) {
    return (
      <p className="text-sm text-[var(--muted)]">This container has not written any output.</p>
    );
  }
  return (
    <>
      <p className="text-xs text-[var(--muted)]">
        {logs.lines.length} lines, fetched at {logs.fetchedAt.toLocaleTimeString()}.
        {logs.truncated &&
          " Output was truncated at 1 MB; fetch fewer lines to see the newest ones."}
      </p>
      <LogView lines={logs.lines} />
    </>
  );
}
