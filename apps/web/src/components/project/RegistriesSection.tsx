import {
  Alert,
  AlertDialog,
  Button,
  Card,
  Description,
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
import { isApiError } from "../../api/client";
import {
  createRegistry,
  deleteRegistry,
  type NewRegistry,
  type Registry,
  registriesQuery,
} from "../../api/registries";
import { useHasRole } from "../../lib/useHasRole";
import { ApiForm } from "../ApiForm";
import { QueryBoundary } from "../QueryBoundary";

type Preset = {
  id: string;
  label: string;
  /** null: the person types the host. */
  host: string | null;
  name: string;
  usernameHint: string;
  tokenHint: string;
};

const presets: Preset[] = [
  {
    id: "ghcr",
    label: "GitHub Container Registry (ghcr.io)",
    host: "ghcr.io",
    name: "GitHub Packages",
    usernameHint: "Your GitHub username.",
    tokenHint: "A personal access token (classic) with the write:packages scope.",
  },
  {
    id: "dockerhub",
    label: "Docker Hub (docker.io)",
    host: "docker.io",
    name: "Docker Hub",
    usernameHint: "Your Docker Hub username.",
    tokenHint: "An access token with Read & Write permission.",
  },
  {
    id: "gitlab",
    label: "GitLab Container Registry (registry.gitlab.com)",
    host: "registry.gitlab.com",
    name: "GitLab Registry",
    usernameHint: "Your GitLab username, or the deploy token's username.",
    tokenHint: "A personal or deploy token with read_registry and write_registry.",
  },
  {
    id: "other",
    label: "Other registry",
    host: null,
    name: "",
    usernameHint: "",
    tokenHint: "Needs permission to push.",
  },
];

export function RegistriesSection({ projectId }: { projectId: string }) {
  const registries = useQuery(registriesQuery(projectId));
  const canManage = useHasRole(projectId, "admin");

  return (
    <section aria-label="Registries" className="flex flex-col gap-3">
      {canManage && (
        <div className="flex justify-end">
          <AddRegistryDialog projectId={projectId} />
        </div>
      )}
      <QueryBoundary
        query={registries}
        label="registries"
        isEmpty={(data) => data.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            No registries yet. Add the registry your images are pushed to, such as ghcr.io or Docker
            Hub, then choose it for each application.
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.map((registry) => (
                <li
                  key={registry.id}
                  className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3"
                >
                  <div className="min-w-48 flex-1">
                    <p className="font-medium">{registry.name}</p>
                    <p className="font-mono text-xs text-[var(--muted)]">
                      {registry.username} @ {registry.host}
                    </p>
                  </div>
                  {canManage && <RemoveRegistry projectId={projectId} registry={registry} />}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </section>
  );
}

function AddRegistryDialog({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [presetId, setPresetId] = useState("ghcr");
  const preset = presets.find((p) => p.id === presetId) ?? presets[0];
  const add = useMutation({
    mutationFn: (input: NewRegistry) => createRegistry(projectId, input),
    onSuccess: (registry) => {
      queryClient.invalidateQueries({ queryKey: registriesQuery(projectId).queryKey });
      toast.success(`${registry.name} added`);
    },
  });

  return (
    <Modal
      onOpenChange={(open) => {
        if (!open) {
          add.reset();
          setPresetId("ghcr");
        }
      }}
    >
      <Button variant="primary">Add registry</Button>
      <Modal.Backdrop>
        <Modal.Container size="sm">
          <Modal.Dialog>
            {({ close }) => {
              const onSubmit = (event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                const data = new FormData(event.currentTarget);
                const text = (name: string) => String(data.get(name) ?? "");
                add.mutate(
                  {
                    name: text("name").trim(),
                    host: preset?.host ?? text("host").trim(),
                    username: text("username").trim(),
                    password: text("password"),
                  },
                  { onSuccess: close },
                );
              };
              return (
                <ApiForm onSubmit={onSubmit} error={add.error} onClearError={add.reset}>
                  <Modal.Header>
                    <Modal.Heading>Add registry</Modal.Heading>
                  </Modal.Header>
                  <Modal.Body className="flex flex-col gap-4">
                    <Select
                      selectedKey={presetId}
                      onSelectionChange={(key) => setPresetId(String(key))}
                    >
                      <Label>Registry</Label>
                      <Select.Trigger>
                        <Select.Value />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover>
                        <ListBox>
                          {presets.map((p) => (
                            <ListBox.Item key={p.id} id={p.id} textValue={p.label}>
                              {p.label}
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                          ))}
                        </ListBox>
                      </Select.Popover>
                    </Select>
                    {preset?.host === null && (
                      <TextField name="host" isRequired>
                        <Label>Registry host</Label>
                        <Input placeholder="registry.example.com:5000" />
                        <Description>The address Docker uses; add :port if needed.</Description>
                        <FieldError />
                      </TextField>
                    )}
                    <TextField
                      key={`name-${presetId}`}
                      name="name"
                      defaultValue={preset?.name}
                      isRequired
                    >
                      <Label>Name</Label>
                      <Input placeholder="How you will recognise it" />
                      <FieldError />
                    </TextField>
                    <TextField name="username" isRequired autoComplete="off">
                      <Label>Username</Label>
                      <Input />
                      {preset?.usernameHint && <Description>{preset.usernameHint}</Description>}
                      <FieldError />
                    </TextField>
                    <TextField name="password" type="password" isRequired autoComplete="off">
                      <Label>Password or access token</Label>
                      <Input />
                      <Description>
                        {preset?.tokenHint} Stored encrypted and never shown again; the first build
                        checks it.
                      </Description>
                      <FieldError />
                    </TextField>
                    {add.error && !isApiError(add.error, 422) && (
                      <Alert status="danger">
                        <Alert.Content>
                          <Alert.Description>{add.error.message}</Alert.Description>
                        </Alert.Content>
                      </Alert>
                    )}
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary" onPress={close}>
                      Cancel
                    </Button>
                    <Button type="submit" variant="primary" isPending={add.isPending}>
                      Add registry
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

function RemoveRegistry({ projectId, registry }: { projectId: string; registry: Registry }) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteRegistry(registry.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: registriesQuery(projectId).queryKey });
      toast.success(`${registry.name} removed`);
    },
    onError: (error) =>
      toast.danger(`Could not remove ${registry.name}`, { description: error.message }),
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
                  <AlertDialog.Heading>Remove {registry.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    ForgeDeck deletes the stored password. Images already pushed stay in the
                    registry. Applications that push here must be changed first.
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
                    Remove registry
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
