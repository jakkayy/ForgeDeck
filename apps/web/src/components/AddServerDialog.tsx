import {
  Alert,
  Button,
  FieldError,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  Spinner,
  TextField,
} from "@heroui/react";
import { useMutation, useQuery } from "@tanstack/react-query";
import type { FormEvent } from "react";
import { isApiError } from "../api/client";
import { projectsQuery } from "../api/projects";
import { createEnrollmentToken, serversQuery } from "../api/servers";
import type { EnrollmentToken } from "../api/types";
import { ApiForm } from "./ApiForm";
import { CopyField } from "./CopyField";

/** Issues an enrollment token and walks the user through installing the Agent. */
export function AddServerDialog() {
  const issue = useMutation({
    mutationFn: ({ projectId, name }: { projectId: string; name: string }) =>
      createEnrollmentToken(projectId, name).then((token) => ({ token, projectId })),
  });

  return (
    <Modal onOpenChange={(open) => !open && issue.reset()}>
      <Button variant="primary">Add server</Button>
      <Modal.Backdrop isDismissable={!issue.isSuccess}>
        <Modal.Container size="md">
          <Modal.Dialog>
            {({ close }) =>
              issue.isSuccess ? (
                <InstallSteps
                  token={issue.data.token}
                  projectId={issue.data.projectId}
                  onDone={close}
                />
              ) : (
                <ServerForm
                  onSubmit={(projectId, name) => issue.mutate({ projectId, name })}
                  onCancel={close}
                  isPending={issue.isPending}
                  error={issue.error}
                  onClearError={issue.reset}
                />
              )
            }
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}

type ServerFormProps = {
  onSubmit: (projectId: string, name: string) => void;
  onCancel: () => void;
  isPending: boolean;
  error: Error | null;
  onClearError: () => void;
};

function ServerForm({ onSubmit, onCancel, isPending, error, onClearError }: ServerFormProps) {
  const projects = useQuery(projectsQuery);
  const manageable =
    projects.data?.items.filter((p) => p.role === "owner" || p.role === "admin") ?? [];

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    onSubmit(String(data.get("project")), String(data.get("server_name")));
  };

  return (
    <ApiForm onSubmit={submit} error={error} onClearError={onClearError}>
      <Modal.Header>
        <Modal.Heading>Add a server</Modal.Heading>
      </Modal.Header>
      <Modal.Body className="flex flex-col gap-4">
        {projects.isSuccess && manageable.length === 0 ? (
          <Alert status="warning">
            <Alert.Content>
              <Alert.Description>
                You need the admin or owner role in a project to add servers. Create a project
                first, or ask a project admin.
              </Alert.Description>
            </Alert.Content>
          </Alert>
        ) : (
          <>
            <Select name="project" placeholder="Choose a project" isRequired>
              <Label>Project</Label>
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {manageable.map((project) => (
                    <ListBox.Item key={project.id} id={project.id} textValue={project.name}>
                      {project.name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
              <FieldError />
            </Select>
            <TextField name="server_name" isRequired>
              <Label>Server name</Label>
              <Input placeholder="production-01" />
              <FieldError />
            </TextField>
          </>
        )}
        {error && !isApiError(error, 422) && (
          <p className="text-sm text-[var(--danger)]">{error.message}</p>
        )}
      </Modal.Body>
      <Modal.Footer>
        <Button variant="secondary" onPress={onCancel}>
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          isPending={isPending}
          isDisabled={manageable.length === 0}
        >
          Create enrollment token
        </Button>
      </Modal.Footer>
    </ApiForm>
  );
}

type InstallStepsProps = { token: EnrollmentToken; projectId: string; onDone: () => void };

function InstallSteps({ token, projectId, onDone }: InstallStepsProps) {
  // Poll quickly only while this dialog waits for the new server.
  const servers = useQuery({ ...serversQuery, refetchInterval: 3_000 });
  const server = servers.data?.items.find(
    (s) => s.project_id === projectId && s.name === token.server_name,
  );
  const connected = server?.status === "online";
  const origin = window.location.origin;

  return (
    <>
      <Modal.Header>
        <Modal.Heading>Install the Agent on {token.server_name}</Modal.Heading>
      </Modal.Header>
      <Modal.Body className="flex flex-col gap-4 text-sm">
        <ol className="flex list-decimal flex-col gap-4 ps-5 marker:text-[var(--muted)]">
          <li className="space-y-2">
            <p>
              Copy the <code className="font-mono">forgedeck-agent</code> binary to the server, then
              enroll it. Paste the token below when asked; it is not echoed or saved in shell
              history.
            </p>
            <CopyField
              label="enroll command"
              value={`sudo forgedeck-agent enroll --url ${origin}`}
            />
            <CopyField label="enrollment token" value={token.token} />
            <p className="text-xs text-[var(--muted)]">
              Shown once. Single use, expires at {new Date(token.expires_at).toLocaleTimeString()}.
            </p>
          </li>
          <li className="space-y-2">
            <p>Start the Agent. It connects out over HTTPS; no inbound port is needed.</p>
            <CopyField label="run command" value="sudo forgedeck-agent run" />
          </li>
        </ol>
        <div
          role="status"
          className="flex items-center gap-2 rounded-lg border border-[var(--separator)] p-3"
        >
          {connected ? (
            <span className="text-[var(--success)]">● {token.server_name} is online.</span>
          ) : (
            <>
              <Spinner size="sm" aria-hidden="true" />
              <span className="text-[var(--muted)]">
                {server
                  ? "Enrolled. Waiting for the first heartbeat…"
                  : "Waiting for the server to enroll…"}
              </span>
            </>
          )}
        </div>
      </Modal.Body>
      <Modal.Footer>
        <Button variant={connected ? "primary" : "secondary"} onPress={onDone}>
          {connected ? "Done" : "Close"}
        </Button>
      </Modal.Footer>
    </>
  );
}
