import { AlertDialog, Button, toast } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { deleteServer, serversQuery } from "../api/servers";
import type { Server } from "../api/types";

/** Removes a server, which revokes its Agent credential immediately. */
export function RemoveServerDialog({ server }: { server: Server }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const remove = useMutation({
    mutationFn: () => deleteServer(server.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ["servers", server.id] });
      queryClient.invalidateQueries({ queryKey: serversQuery.queryKey });
      toast.success(`${server.name} was removed`);
      navigate({ to: "/servers" });
    },
    onError: (error) =>
      toast.danger(`Could not remove ${server.name}`, { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="danger-soft" isPending={remove.isPending}>
        Remove server
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Remove {server.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body className="flex flex-col gap-2 text-sm">
                  <p>The Agent's credential is revoked immediately and it stops receiving jobs.</p>
                  <p className="text-[var(--muted)]">
                    Containers on the host keep running. To manage the host again, add it as a new
                    server and enroll the Agent again.
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
                    Remove server
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
