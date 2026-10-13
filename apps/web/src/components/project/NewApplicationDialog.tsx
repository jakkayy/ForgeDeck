import {
  Alert,
  Button,
  Description,
  Disclosure,
  FieldError,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  TextField,
  toast,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { applicationsQuery, createApplication, type NewApplication } from "../../api/applications";
import { isApiError } from "../../api/client";
import { gitConnectionsQuery, repositoriesQuery } from "../../api/git";
import { ApiForm } from "../ApiForm";

/** Creates an application: connection → repository → branch, with build settings tucked away. */
export function NewApplicationDialog({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: (input: NewApplication) => createApplication(projectId, input),
    onSuccess: (app) => {
      queryClient.invalidateQueries({ queryKey: applicationsQuery(projectId).queryKey });
      toast.success(`${app.name} builds ${app.repository_full_name} @ ${app.branch} on every push`);
    },
  });

  return (
    <Modal onOpenChange={(open) => !open && create.reset()}>
      <Button variant="primary">New application</Button>
      <Modal.Backdrop>
        <Modal.Container size="md" scroll="inside">
          <Modal.Dialog>
            {({ close }) => (
              <ApplicationForm
                projectId={projectId}
                isPending={create.isPending}
                error={create.error}
                onClearError={create.reset}
                onCancel={close}
                onSubmit={(input) => create.mutate(input, { onSuccess: close })}
              />
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}

type FormProps = {
  projectId: string;
  isPending: boolean;
  error: Error | null;
  onClearError: () => void;
  onCancel: () => void;
  onSubmit: (input: NewApplication) => void;
};

function ApplicationForm({
  projectId,
  isPending,
  error,
  onClearError,
  onCancel,
  onSubmit,
}: FormProps) {
  const connections = useQuery(gitConnectionsQuery(projectId));
  const [connectionId, setConnectionId] = useState<string | null>(null);
  const repositories = useQuery({
    ...repositoriesQuery(connectionId ?? ""),
    enabled: connectionId !== null,
  });
  const [repositoryId, setRepositoryId] = useState<string | null>(null);
  const [branch, setBranch] = useState("");

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const text = (name: string) => String(data.get(name) ?? "").trim();
    onSubmit({
      name: text("name"),
      git_connection_id: connectionId ?? "",
      repository_id: repositoryId ?? "",
      branch: branch.trim() || undefined,
      dockerfile_path: text("dockerfile_path") || undefined,
      build_context: text("build_context") || undefined,
      test_command: text("test_command") || undefined,
    });
  };

  const noConnections = connections.isSuccess && connections.data.length === 0;
  return (
    <ApiForm onSubmit={submit} error={error} onClearError={onClearError}>
      <Modal.Header>
        <Modal.Heading>New application</Modal.Heading>
      </Modal.Header>
      <Modal.Body className="flex flex-col gap-4">
        {noConnections ? (
          <Alert status="warning">
            <Alert.Content>
              <Alert.Description>Connect GitHub or GitLab in the Git tab first.</Alert.Description>
            </Alert.Content>
          </Alert>
        ) : (
          <>
            <TextField name="name" isRequired autoFocus>
              <Label>Name</Label>
              <Input placeholder="API" />
              <FieldError />
            </TextField>
            <Select
              placeholder="Choose a connection"
              isRequired
              selectedKey={connectionId}
              onSelectionChange={(key) => {
                setConnectionId(String(key));
                setRepositoryId(null);
                setBranch("");
              }}
            >
              <Label>Git connection</Label>
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {(connections.data ?? []).map((conn) => (
                    <ListBox.Item key={conn.id} id={conn.id} textValue={conn.display_name}>
                      {conn.display_name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
            <Select
              name="repository_id"
              placeholder={
                repositories.isFetching ? "Loading repositories…" : "Choose a repository"
              }
              isRequired
              isDisabled={connectionId === null || !repositories.isSuccess}
              selectedKey={repositoryId}
              onSelectionChange={(key) => {
                const repo = repositories.data?.find((r) => r.id === key);
                setRepositoryId(String(key));
                setBranch(repo?.default_branch ?? "");
              }}
            >
              <Label>Repository</Label>
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {(repositories.data ?? []).map((repo) => (
                    <ListBox.Item key={repo.id} id={repo.id} textValue={repo.full_name}>
                      {repo.full_name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
              <FieldError />
            </Select>
            {repositories.isError && (
              <p className="text-sm text-[var(--danger)]">{repositories.error.message}</p>
            )}
            <TextField name="branch" value={branch} onChange={setBranch} isRequired>
              <Label>Branch</Label>
              <Input placeholder="main" />
              <Description>Every push to this branch builds a new image.</Description>
              <FieldError />
            </TextField>
            <Disclosure>
              <Disclosure.Heading>
                <Disclosure.Trigger className="flex items-center gap-1 text-sm">
                  Build settings
                  <Disclosure.Indicator />
                </Disclosure.Trigger>
              </Disclosure.Heading>
              <Disclosure.Content>
                <Disclosure.Body className="flex flex-col gap-4 pt-3">
                  <TextField name="dockerfile_path" defaultValue="Dockerfile">
                    <Label>Dockerfile</Label>
                    <Input />
                    <FieldError />
                  </TextField>
                  <TextField name="build_context" defaultValue=".">
                    <Label>Build context</Label>
                    <Input />
                    <Description>
                      Directory sent to the build, relative to the repository root.
                    </Description>
                    <FieldError />
                  </TextField>
                  <TextField name="test_command">
                    <Label>Test command</Label>
                    <Input placeholder="npm test" />
                    <Description>
                      Optional. Runs in the Runner before the image is built.
                    </Description>
                    <FieldError />
                  </TextField>
                </Disclosure.Body>
              </Disclosure.Content>
            </Disclosure>
          </>
        )}
        {error && !isApiError(error, 422) && (
          <Alert status="danger">
            <Alert.Content>
              <Alert.Description>{error.message}</Alert.Description>
            </Alert.Content>
          </Alert>
        )}
      </Modal.Body>
      <Modal.Footer>
        <Button variant="secondary" onPress={onCancel}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" isPending={isPending} isDisabled={noConnections}>
          Create application
        </Button>
      </Modal.Footer>
    </ApiForm>
  );
}
