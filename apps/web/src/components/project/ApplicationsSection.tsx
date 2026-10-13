import { AlertDialog, Button, Card, toast } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type Application, applicationsQuery, deleteApplication } from "../../api/applications";
import { useHasRole } from "../../lib/useHasRole";
import { QueryBoundary } from "../QueryBoundary";
import { NewApplicationDialog } from "./NewApplicationDialog";

/** The project's applications and what each builds from. */
export function ApplicationsSection({ projectId }: { projectId: string }) {
  const apps = useQuery(applicationsQuery(projectId));
  const canManage = useHasRole(projectId, "admin");

  return (
    <section aria-label="Applications" className="flex flex-col gap-3">
      {canManage && (
        <div className="flex justify-end">
          <NewApplicationDialog projectId={projectId} />
        </div>
      )}
      <QueryBoundary
        query={apps}
        label="applications"
        isEmpty={(data) => data.items.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            No applications yet. An application builds a Docker image from one repository and branch
            on every push, and pushes it to a registry.
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.items.map((app) => (
                <li key={app.id} className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
                  <div className="min-w-48 flex-1">
                    <Link
                      to="/applications/$applicationId"
                      params={{ applicationId: app.id }}
                      className="font-medium hover:underline"
                    >
                      {app.name}
                    </Link>
                    <p className="truncate font-mono text-xs text-[var(--muted)]">
                      {app.repository_full_name} @ {app.branch}
                    </p>
                  </div>
                  <p className="hidden truncate font-mono text-xs text-[var(--muted)] sm:block">
                    {app.build_context === "."
                      ? app.dockerfile_path
                      : `${app.build_context}/${app.dockerfile_path}`}
                  </p>
                  {canManage && <DeleteApplication projectId={projectId} app={app} />}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </section>
  );
}

function DeleteApplication({ projectId, app }: { projectId: string; app: Application }) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteApplication(app.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: applicationsQuery(projectId).queryKey });
      toast.success(`${app.name} was deleted`);
    },
    onError: (error) =>
      toast.danger(`Could not delete ${app.name}`, { description: error.message }),
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
                  <AlertDialog.Heading>Delete {app.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body>
                  <p className="text-sm">
                    Pushes to {app.repository_full_name} stop building, and its build history is
                    deleted. The repository itself is not touched.
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
                    Delete application
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
