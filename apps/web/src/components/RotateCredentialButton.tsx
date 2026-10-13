import { AlertDialog, Button, toast } from "@heroui/react";
import { useMutation } from "@tanstack/react-query";
import { requestCredentialRotation } from "../api/servers";
import type { Server } from "../api/types";

/** Asks the Agent to replace its credential. The Agent does it within a heartbeat or two. */
export function RotateCredentialButton({ server }: { server: Server }) {
  const rotate = useMutation({
    mutationFn: () => requestCredentialRotation(server.id),
    onSuccess: () =>
      toast.success(`${server.name} will rotate its credential`, {
        description:
          server.status === "online"
            ? "The Agent does it at its next heartbeat, within about 20 seconds."
            : "The Agent does it when it next connects.",
      }),
    onError: (error) =>
      toast.danger("Could not request the rotation", { description: error.message }),
  });

  return (
    <AlertDialog>
      <Button size="sm" variant="secondary" isPending={rotate.isPending}>
        Rotate credential
      </Button>
      <AlertDialog.Backdrop>
        <AlertDialog.Container size="sm">
          <AlertDialog.Dialog>
            {({ close }) => (
              <>
                <AlertDialog.Header>
                  <AlertDialog.Heading>Rotate the credential of {server.name}?</AlertDialog.Heading>
                </AlertDialog.Header>
                <AlertDialog.Body className="flex flex-col gap-2 text-sm">
                  <p>
                    The Agent gets a new credential and saves it on the server by itself; nothing
                    needs to be copied. The old one keeps working for 15 minutes in case something
                    goes wrong.
                  </p>
                  <p className="text-[var(--muted)]">
                    Do this when the credential may have been exposed. It is also replaced
                    automatically after 90 days.
                  </p>
                </AlertDialog.Body>
                <AlertDialog.Footer>
                  <Button variant="secondary" onPress={close}>
                    Cancel
                  </Button>
                  <Button
                    variant="primary"
                    onPress={() => {
                      rotate.mutate();
                      close();
                    }}
                  >
                    Rotate credential
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
