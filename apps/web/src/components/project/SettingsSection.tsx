import {
  AlertDialog,
  Button,
  Card,
  FieldError,
  Input,
  Label,
  TextField,
  toast,
} from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import { isApiError } from "../../api/client";
import { deleteProject, projectQuery, renameProject } from "../../api/projects";
import type { Project } from "../../api/types";
import { useHasRole } from "../../lib/useHasRole";
import { ApiForm } from "../ApiForm";

/** Rename (admins) and delete (owners) the project. */
export function SettingsSection({ project }: { project: Project }) {
  const isOwner = useHasRole(project.id, "owner");
  return (
    <section aria-label="Settings" className="flex max-w-xl flex-col gap-6">
      <RenameProject project={project} />
      {isOwner && <DeleteProject project={project} />}
    </section>
  );
}

function RenameProject({ project }: { project: Project }) {
  const queryClient = useQueryClient();
  const rename = useMutation({
    mutationFn: (name: string) => renameProject(project.id, name),
    onSuccess: (updated) => {
      queryClient.setQueryData(projectQuery(project.id).queryKey, updated);
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      toast.success("Project renamed");
    },
  });

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    rename.mutate(String(new FormData(event.currentTarget).get("name")));
  };

  return (
    <Card className="gap-3 p-4">
      <h2 className="text-base font-semibold">Name</h2>
      <ApiForm
        className="flex flex-col gap-3"
        onSubmit={onSubmit}
        error={rename.error}
        onClearError={rename.reset}
      >
        <TextField name="name" defaultValue={project.name} isRequired>
          <Label>Project name</Label>
          <Input />
          <FieldError />
        </TextField>
        {rename.error && !isApiError(rename.error, 422) && (
          <p className="text-sm text-[var(--danger)]">{rename.error.message}</p>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={rename.isPending}>
            Save name
          </Button>
        </div>
      </ApiForm>
    </Card>
  );
}

function DeleteProject({ project }: { project: Project }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [typed, setTyped] = useState("");
  const remove = useMutation({
    mutationFn: () => deleteProject(project.id, typed),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ["projects", project.id] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      toast.success(`${project.name} was deleted`);
      navigate({ to: "/projects" });
    },
  });

  const resetDialog = () => {
    setTyped("");
    remove.reset();
  };

  return (
    <Card className="gap-3 border border-[var(--danger)] p-4">
      <h2 className="text-base font-semibold text-[var(--danger)]">Delete this project</h2>
      <p className="text-sm text-[var(--muted)]">
        Removes its applications, environments, releases, build history, secrets, registries, Git
        connections and audit log for good. Servers must be removed first, and containers already
        running on them keep running. This cannot be undone; restore a backup to get it back.
      </p>
      <div>
        <AlertDialog onOpenChange={resetDialog}>
          <Button variant="danger-soft">Delete project</Button>
          <AlertDialog.Backdrop>
            <AlertDialog.Container size="sm">
              <AlertDialog.Dialog>
                {({ close }) => (
                  <>
                    <AlertDialog.Header>
                      <AlertDialog.Heading>Delete {project.name}?</AlertDialog.Heading>
                    </AlertDialog.Header>
                    <AlertDialog.Body className="flex flex-col gap-3 text-sm">
                      <TextField value={typed} onChange={setTyped} autoFocus>
                        <Label>
                          Type <code className="font-mono">{project.slug}</code> to confirm
                        </Label>
                        <Input autoComplete="off" />
                      </TextField>
                      {remove.error && (
                        <p role="alert" className="text-[var(--danger)]">
                          {remove.error.message}
                        </p>
                      )}
                    </AlertDialog.Body>
                    <AlertDialog.Footer>
                      <Button variant="secondary" onPress={close}>
                        Cancel
                      </Button>
                      <Button
                        variant="danger"
                        isDisabled={typed !== project.slug}
                        isPending={remove.isPending}
                        onPress={() => remove.mutate()}
                      >
                        Delete project
                      </Button>
                    </AlertDialog.Footer>
                  </>
                )}
              </AlertDialog.Dialog>
            </AlertDialog.Container>
          </AlertDialog.Backdrop>
        </AlertDialog>
      </div>
    </Card>
  );
}
