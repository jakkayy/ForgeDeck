import { AlertDialog, Button, Card, Chip, toast } from "@heroui/react";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { type Release, releaseListQuery, rollbackRelease } from "../../api/environments";
import { loadedItems } from "../../api/paged";
import { shortImage } from "../../lib/serviceConfig";
import { timeAgo } from "../../lib/time";
import { useHasRole } from "../../lib/useHasRole";
import { LoadMore } from "../LoadMore";
import { QueryBoundary } from "../QueryBoundary";

type Props = { environmentId: string; projectId: string };

export function ReleaseHistory({ environmentId, projectId }: Props) {
  const releases = useInfiniteQuery(releaseListQuery(environmentId));
  const canDeploy = useHasRole(projectId, "developer");

  return (
    <section aria-label="Releases" className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Release history</h2>
      <QueryBoundary
        query={releases}
        label="releases"
        isEmpty={(data) => loadedItems(data).length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            No releases yet. A release is recorded when a deployment passes its health check.
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {loadedItems(data).map((release) => (
                <li
                  key={release.id}
                  className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3"
                >
                  <p className="min-w-48 flex-1 font-mono text-sm">
                    <code>{release.commit_sha.slice(0, 7)}</code> · {shortImage(release.image_ref)}
                  </p>
                  {release.current && (
                    <Chip size="sm" variant="soft" color="success">
                      current
                    </Chip>
                  )}
                  <p className="w-28 text-sm text-[var(--muted)]">{timeAgo(release.created_at)}</p>
                  {canDeploy && !release.current && (
                    <RollbackButton release={release} environmentId={environmentId} />
                  )}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
      <LoadMore query={releases} />
    </section>
  );
}

function RollbackButton({ release, environmentId }: { release: Release; environmentId: string }) {
  const queryClient = useQueryClient();
  const rollback = useMutation({
    mutationFn: () => rollbackRelease(release.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["environments", environmentId] });
      toast.success(`Rolling back to ${release.commit_sha.slice(0, 7)}`);
    },
    onError: (error) =>
      toast.danger("Could not start the rollback", { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="secondary" isPending={rollback.isPending}>
        Roll back
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>
                    Roll back to {release.commit_sha.slice(0, 7)}?
                  </AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body className="flex flex-col gap-2 text-sm">
                  <p>
                    ForgeDeck redeploys that exact image,{" "}
                    <code className="font-mono">{shortImage(release.image_ref)}</code>, with the
                    configuration it ran with. Nothing is rebuilt. The service restarts, so there is
                    a short gap.
                  </p>
                  <p className="font-medium">Data is not rolled back.</p>
                  <p>
                    Volumes keep what newer versions wrote to them, and database changes they made
                    stay. If a newer version migrated data, the old version may not be able to use
                    it.
                  </p>
                </AlertDialog.Body>
                <AlertDialog.Footer>
                  <Button variant="secondary" onPress={close}>
                    Cancel
                  </Button>
                  <Button
                    variant="danger"
                    onPress={() => {
                      rollback.mutate();
                      close();
                    }}
                  >
                    Roll back
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
