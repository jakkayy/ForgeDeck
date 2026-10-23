import { Alert, Button, FieldError, Input, Label, Modal, Spinner, TextField } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { FormEvent } from "react";
import { isApiError } from "../api/client";
import { type CreatedRunner, createRunner, runnersQuery } from "../api/runners";
import { ApiForm } from "./ApiForm";
import { CopyField } from "./CopyField";

/** Where the installer comes from: the project's latest GitHub release. */
const INSTALLER_URL =
  "https://github.com/jakkayy/ForgeDeck/releases/latest/download/install-runner.sh";

function installCommand(origin: string, credential: string) {
  return `curl -fsSL ${INSTALLER_URL} -o install-runner.sh && sudo FORGEDECK_RUNNER_CREDENTIAL=${credential} bash install-runner.sh --url ${origin}`;
}

/** Creates a Runner and walks the administrator through installing it on a build host. */
export function AddRunnerDialog() {
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: createRunner,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: runnersQuery.queryKey }),
  });

  return (
    <Modal onOpenChange={(open) => !open && create.reset()}>
      <Button variant="primary">Add Runner</Button>
      <Modal.Backdrop isDismissable={!create.isSuccess}>
        <Modal.Container size="md">
          <Modal.Dialog>
            {({ close }) =>
              create.isSuccess ? (
                <InstallSteps runner={create.data} onDone={close} />
              ) : (
                <RunnerForm
                  onSubmit={(name) => create.mutate(name)}
                  onCancel={close}
                  isPending={create.isPending}
                  error={create.error}
                  onClearError={create.reset}
                />
              )
            }
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}

type FormProps = {
  onSubmit: (name: string) => void;
  onCancel: () => void;
  isPending: boolean;
  error: Error | null;
  onClearError: () => void;
};

function RunnerForm({ onSubmit, onCancel, isPending, error, onClearError }: FormProps) {
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onSubmit(String(new FormData(event.currentTarget).get("name")));
  };
  return (
    <ApiForm onSubmit={submit} error={error} onClearError={onClearError}>
      <Modal.Header>
        <Modal.Heading>Add a Runner</Modal.Heading>
      </Modal.Header>
      <Modal.Body className="flex flex-col gap-4">
        <p className="text-sm text-[var(--muted)]">
          A Runner builds your images. It runs the code of your repositories, so install it on a
          dedicated build host rather than on a server that runs your applications.
        </p>
        <TextField name="name" defaultValue="build-01" isRequired autoFocus>
          <Label>Name</Label>
          <Input placeholder="build-01" />
          <FieldError />
        </TextField>
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
        <Button type="submit" variant="primary" isPending={isPending}>
          Create Runner
        </Button>
      </Modal.Footer>
    </ApiForm>
  );
}

function InstallSteps({ runner, onDone }: { runner: CreatedRunner; onDone: () => void }) {
  const runners = useQuery({ ...runnersQuery, refetchInterval: 3_000 });
  const live = runners.data?.find((r) => r.id === runner.id);
  const connected = live?.status === "online";

  return (
    <>
      <Modal.Header>
        <Modal.Heading>Install the Runner {runner.name}</Modal.Heading>
      </Modal.Header>
      <Modal.Body className="flex flex-col gap-4 text-sm">
        <div className="space-y-2">
          <p>
            Run this on the build host (Debian or Ubuntu with Docker's apt repository). It installs
            the Runner, gives it its own rootless Docker daemon, and starts it.
          </p>
          <CopyField
            label="install command"
            value={installCommand(window.location.origin, runner.credential)}
          />
          <p className="text-xs text-[var(--muted)]">
            The command contains the credential, which is shown once and stays in the shell history
            of that host. If it leaks or is lost, delete this Runner and add another.
          </p>
        </div>
        <div
          role="status"
          className="flex items-center gap-2 rounded-lg border border-[var(--separator)] p-3"
        >
          {connected ? (
            <span className="text-[var(--success)]">● {runner.name} is online.</span>
          ) : (
            <>
              <Spinner size="sm" aria-hidden="true" />
              <span className="text-[var(--muted)]">Waiting for the Runner to connect…</span>
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
