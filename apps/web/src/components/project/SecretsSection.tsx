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
import { deleteSecret, type Secret, secretsQuery, setSecret } from "../../api/secrets";
import { timeAgo } from "../../lib/time";
import { useHasRole } from "../../lib/useHasRole";
import { ApiForm } from "../ApiForm";
import { QueryBoundary } from "../QueryBoundary";

/** Project secrets: named values that services receive at deployment. Values are write-only. */
export function SecretsSection({ projectId }: { projectId: string }) {
  const secrets = useQuery(secretsQuery(projectId));
  const canManage = useHasRole(projectId, "admin");

  return (
    <section aria-label="Secrets" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="max-w-xl text-sm text-[var(--muted)]">
          Secrets are stored encrypted and can never be read back, only replaced. An environment
          receives the ones it lists as environment variables at its next deployment.
        </p>
        {canManage && <SecretDialog projectId={projectId} />}
      </div>
      <QueryBoundary
        query={secrets}
        label="secrets"
        isEmpty={(data) => data.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            No secrets yet. Add database passwords and API keys here instead of in an environment's
            variables, which every project member can read.
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.map((secret) => (
                <li
                  key={secret.name}
                  className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3"
                >
                  <div className="min-w-48 flex-1">
                    <p className="font-mono text-sm font-medium">{secret.name}</p>
                    <p className="text-xs text-[var(--muted)]">
                      Version {secret.version} · changed {timeAgo(secret.updated_at)}
                    </p>
                  </div>
                  {canManage && (
                    <div className="flex gap-2">
                      <SecretDialog projectId={projectId} secret={secret} />
                      <DeleteSecret projectId={projectId} secret={secret} />
                    </div>
                  )}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </section>
  );
}

/** Adds a secret, or replaces the value of an existing one. */
function SecretDialog({ projectId, secret }: { projectId: string; secret?: Secret }) {
  const queryClient = useQueryClient();
  const save = useMutation({
    mutationFn: ({ name, value }: { name: string; value: string }) =>
      setSecret(projectId, name, value),
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: secretsQuery(projectId).queryKey });
      toast.success(
        saved.version === 1
          ? `${saved.name} added`
          : `${saved.name} replaced (version ${saved.version})`,
      );
    },
  });

  return (
    <Modal onOpenChange={(open) => !open && save.reset()}>
      <Button size={secret ? "sm" : "md"} variant={secret ? "secondary" : "primary"}>
        {secret ? "Replace" : "Add secret"}
      </Button>
      <Modal.Backdrop>
        <Modal.Container size="sm">
          <Modal.Dialog>
            {({ close }) => {
              const onSubmit = (event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                const data = new FormData(event.currentTarget);
                save.mutate(
                  {
                    name: secret?.name ?? String(data.get("name") ?? "").trim(),
                    value: String(data.get("value") ?? ""),
                  },
                  { onSuccess: close },
                );
              };
              return (
                <ApiForm onSubmit={onSubmit} error={save.error} onClearError={save.reset}>
                  <Modal.Header>
                    <Modal.Heading>
                      {secret ? `Replace ${secret.name}` : "Add secret"}
                    </Modal.Heading>
                  </Modal.Header>
                  <Modal.Body className="flex flex-col gap-4">
                    {!secret && (
                      <TextField name="name" isRequired autoFocus autoComplete="off">
                        <Label>Name</Label>
                        <Input placeholder="DATABASE_PASSWORD" />
                        <Description>
                          Capital letters, digits and underscores. The service receives it in an
                          environment variable of this name.
                        </Description>
                        <FieldError />
                      </TextField>
                    )}
                    <TextField
                      name="value"
                      type="password"
                      isRequired
                      autoFocus={!!secret}
                      autoComplete="off"
                    >
                      <Label>Value</Label>
                      <Input />
                      <Description>
                        At least 6 characters. It is never shown again. A replaced value reaches
                        each service at its next deployment.
                      </Description>
                      <FieldError />
                    </TextField>
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
                      {secret ? "Replace" : "Add secret"}
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

function DeleteSecret({ projectId, secret }: { projectId: string; secret: Secret }) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteSecret(projectId, secret.name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: secretsQuery(projectId).queryKey });
      toast.success(`${secret.name} was deleted`);
    },
    onError: (error) =>
      toast.danger(`Could not delete ${secret.name}`, { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="danger-soft" isPending={remove.isPending}>
        Delete
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Delete {secret.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    The value is deleted and cannot be recovered. Services that already run keep the
                    value they received until they are deployed again. A secret an environment still
                    lists cannot be deleted.
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
                    Delete secret
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
