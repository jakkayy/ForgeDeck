import {
  AlertDialog,
  Button,
  Card,
  FieldError,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  Spinner,
  TextField,
  toast,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import type { FormEvent } from "react";
import { meQuery } from "../../../api/auth";
import { isApiError } from "../../../api/client";
import { addMember, membersQuery, projectQuery, removeMember } from "../../../api/projects";
import type { Member, Role } from "../../../api/types";
import { ApiForm } from "../../../components/ApiForm";
import { QueryError } from "../../../components/QueryError";
import { RoleChip } from "../../../components/RoleChip";
import { useHasRole } from "../../../lib/useHasRole";

export const Route = createFileRoute("/_app/projects/$projectId")({
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(projectQuery(params.projectId)),
      context.queryClient.prefetchQuery(membersQuery(params.projectId)),
    ]),
  component: ProjectPage,
});

const roles: { id: Role; description: string }[] = [
  { id: "viewer", description: "View servers, containers and logs" },
  { id: "developer", description: "Also start, stop and restart containers" },
  { id: "admin", description: "Also manage servers and members" },
  { id: "owner", description: "Everything, including deleting the project" },
];

function ProjectPage() {
  const { projectId } = Route.useParams();
  const project = useQuery(projectQuery(projectId));
  const members = useQuery(membersQuery(projectId));
  const canManage = useHasRole(projectId, "admin");

  if (project.isPending) {
    return <Spinner aria-label="Loading project" />;
  }
  if (project.isError) {
    return <QueryError error={project.error} onRetry={() => project.refetch()} />;
  }
  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link to="/projects" className="text-sm text-[var(--muted)] hover:text-[var(--foreground)]">
          ← Projects
        </Link>
        <div className="mt-1 flex flex-wrap items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">{project.data.name}</h1>
          <RoleChip role={project.data.role} />
        </div>
      </div>
      <section aria-labelledby="members-heading" className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4">
          <h2 id="members-heading" className="font-semibold">
            Members
          </h2>
          {canManage && <AddMemberDialog projectId={projectId} />}
        </div>
        {members.isPending && <Spinner aria-label="Loading members" />}
        {members.isError && <QueryError error={members.error} onRetry={() => members.refetch()} />}
        {members.isSuccess && (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {members.data.items.map((member) => (
                <MemberRow
                  key={member.user_id}
                  projectId={projectId}
                  member={member}
                  canManage={canManage}
                />
              ))}
            </ul>
          </Card>
        )}
      </section>
    </div>
  );
}

function MemberRow({
  projectId,
  member,
  canManage,
}: {
  projectId: string;
  member: Member;
  canManage: boolean;
}) {
  const { data: me } = useSuspenseQuery(meQuery);
  const isSelf = me.id === member.user_id;
  return (
    <li className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
      <p className="min-w-48 flex-1 truncate">
        {member.email}
        {isSelf && <span className="text-[var(--muted)]"> (you)</span>}
      </p>
      <RoleChip role={member.role} />
      {(canManage || isSelf) && (
        <RemoveMember projectId={projectId} member={member} isSelf={isSelf} />
      )}
    </li>
  );
}

function RemoveMember({
  projectId,
  member,
  isSelf,
}: {
  projectId: string;
  member: Member;
  isSelf: boolean;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => removeMember(projectId, member.user_id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      toast.success(isSelf ? "You left the project" : `${member.email} was removed`);
    },
    onError: (error) => toast.danger("Could not remove member", { description: error.message }),
  });
  const action = isSelf ? "Leave" : "Remove";

  return (
    <AlertDialog>
      <Button size="sm" variant="danger-soft" isPending={remove.isPending}>
        {action}
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>
                    {isSelf ? "Leave this project?" : `Remove ${member.email}?`}
                  </AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    {isSelf
                      ? "You lose access to its servers immediately."
                      : "They lose access to this project's servers immediately."}
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
                    {action}
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

function AddMemberDialog({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const add = useMutation({
    mutationFn: ({ email, role }: { email: string; role: Role }) =>
      addMember(projectId, email, role),
    onSuccess: (member) => {
      queryClient.invalidateQueries({ queryKey: membersQuery(projectId).queryKey });
      toast.success(`${member.email} added as ${member.role}`);
    },
  });

  return (
    <Modal onOpenChange={(open) => !open && add.reset()}>
      <Button size="sm" variant="primary">
        Add member
      </Button>
      <Modal.Backdrop>
        <Modal.Container size="sm">
          <Modal.Dialog>
            {({ close }) => {
              const onSubmit = (event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                const data = new FormData(event.currentTarget);
                add.mutate(
                  { email: String(data.get("email")), role: String(data.get("role")) as Role },
                  { onSuccess: close },
                );
              };
              return (
                <ApiForm onSubmit={onSubmit} error={add.error} onClearError={add.reset}>
                  <Modal.Header>
                    <Modal.Heading>Add member</Modal.Heading>
                  </Modal.Header>
                  <Modal.Body className="flex flex-col gap-4">
                    <TextField name="email" type="email" isRequired autoFocus>
                      <Label>Email</Label>
                      <Input placeholder="teammate@example.com" />
                      <FieldError />
                    </TextField>
                    <Select name="role" defaultSelectedKey="developer" isRequired>
                      <Label>Role</Label>
                      <Select.Trigger>
                        <Select.Value />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover>
                        <ListBox>
                          {roles.map((role) => (
                            <ListBox.Item key={role.id} id={role.id} textValue={role.id}>
                              <div className="flex flex-col">
                                <span className="capitalize">{role.id}</span>
                                <span className="text-xs text-[var(--muted)]">
                                  {role.description}
                                </span>
                              </div>
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                          ))}
                        </ListBox>
                      </Select.Popover>
                      <FieldError />
                    </Select>
                    {add.error && !isApiError(add.error, 422) && (
                      <p className="text-sm text-[var(--danger)]">{add.error.message}</p>
                    )}
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary" onPress={close}>
                      Cancel
                    </Button>
                    <Button type="submit" variant="primary" isPending={add.isPending}>
                      Add member
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
