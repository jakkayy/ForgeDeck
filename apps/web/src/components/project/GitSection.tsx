import {
  Alert,
  AlertDialog,
  Button,
  Card,
  Description,
  FieldError,
  Input,
  Label,
  Modal,
  TextField,
  toast,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { FormEvent } from "react";
import { isApiError } from "../../api/client";
import {
  connectGitLab,
  deleteGitConnection,
  type GitConnection,
  gitConnectionsQuery,
  startGitHubInstall,
} from "../../api/git";
import { useHasRole } from "../../lib/useHasRole";
import { ApiForm } from "../ApiForm";
import { QueryBoundary } from "../QueryBoundary";

const providerLabel = { github: "GitHub", gitlab: "GitLab" } as const;

export function GitSection({ projectId }: { projectId: string }) {
  const connections = useQuery(gitConnectionsQuery(projectId));
  const canManage = useHasRole(projectId, "admin");

  return (
    <section aria-label="Git connections" className="flex flex-col gap-3">
      {canManage && (
        <div className="flex flex-wrap justify-end gap-2">
          <ConnectGitHubButton projectId={projectId} />
          <ConnectGitLabDialog projectId={projectId} />
        </div>
      )}
      <QueryBoundary
        query={connections}
        label="connections"
        isEmpty={(data) => data.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            No Git provider connected. Connect GitHub or GitLab so applications can build from your
            repositories.
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.map((conn) => (
                <li key={conn.id} className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
                  <div className="min-w-48 flex-1">
                    <p className="font-medium">{conn.display_name}</p>
                    <p className="text-xs text-[var(--muted)]">
                      {providerLabel[conn.provider]} · {conn.base_url}
                    </p>
                  </div>
                  {canManage && <RemoveConnection projectId={projectId} connection={conn} />}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </section>
  );
}

function ConnectGitHubButton({ projectId }: { projectId: string }) {
  const start = useMutation({
    mutationFn: () => startGitHubInstall(projectId),
    // GitHub sends the user back to this page with ?git=connected or ?git_error=.
    onSuccess: ({ install_url }) => window.location.assign(install_url),
    onError: (error) =>
      toast.danger("Could not start the GitHub connection", {
        description: isApiError(error, 409)
          ? "The GitHub App is not configured on this ForgeDeck. Ask an administrator, or connect GitLab."
          : error.message,
      }),
  });
  return (
    <Button variant="secondary" isPending={start.isPending} onPress={() => start.mutate()}>
      Connect GitHub
    </Button>
  );
}

function ConnectGitLabDialog({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const connect = useMutation({
    mutationFn: ({ baseUrl, token }: { baseUrl: string; token: string }) =>
      connectGitLab(projectId, baseUrl, token),
    onSuccess: (conn) => {
      queryClient.invalidateQueries({ queryKey: gitConnectionsQuery(projectId).queryKey });
      toast.success(`Connected GitLab as ${conn.display_name}`);
    },
  });

  return (
    <Modal onOpenChange={(open) => !open && connect.reset()}>
      <Button variant="secondary">Connect GitLab</Button>
      <Modal.Backdrop>
        <Modal.Container size="sm">
          <Modal.Dialog>
            {({ close }) => {
              const onSubmit = (event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                const data = new FormData(event.currentTarget);
                connect.mutate(
                  { baseUrl: String(data.get("base_url")), token: String(data.get("token")) },
                  { onSuccess: close },
                );
              };
              return (
                <ApiForm onSubmit={onSubmit} error={connect.error} onClearError={connect.reset}>
                  <Modal.Header>
                    <Modal.Heading>Connect GitLab</Modal.Heading>
                  </Modal.Header>
                  <Modal.Body className="flex flex-col gap-4">
                    <TextField name="base_url" defaultValue="https://gitlab.com" isRequired>
                      <Label>GitLab address</Label>
                      <Input />
                      <FieldError />
                    </TextField>
                    <TextField name="token" type="password" autoComplete="off" isRequired autoFocus>
                      <Label>Access token</Label>
                      <Input placeholder="glpat-…" />
                      <Description>
                        A project or group access token with the api scope and Maintainer role. It
                        is stored encrypted and never shown again.
                      </Description>
                      <FieldError />
                    </TextField>
                    {connect.error && !isApiError(connect.error, 422) && (
                      <Alert status="danger">
                        <Alert.Content>
                          <Alert.Description>{connect.error.message}</Alert.Description>
                        </Alert.Content>
                      </Alert>
                    )}
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary" onPress={close}>
                      Cancel
                    </Button>
                    <Button type="submit" variant="primary" isPending={connect.isPending}>
                      Connect
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

function RemoveConnection({
  projectId,
  connection,
}: {
  projectId: string;
  connection: GitConnection;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteGitConnection(connection.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gitConnectionsQuery(projectId).queryKey });
      toast.success(`${connection.display_name} disconnected`);
    },
    onError: (error) =>
      toast.danger("Could not remove the connection", { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="danger-soft" isPending={remove.isPending}>
        Disconnect
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Disconnect {connection.display_name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    ForgeDeck deletes the stored credentials. Applications that build from this
                    connection must be removed first.
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
                    Disconnect
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
