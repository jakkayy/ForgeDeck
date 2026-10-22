import { Form } from "@heroui/react";
import type { ComponentProps } from "react";
import { isApiError } from "../api/client";

type Props = Omit<ComponentProps<typeof Form>, "validationErrors"> & {
  error: Error | null;
  onClearError: () => void;
};

/** A Form that shows API validation errors on their fields and clears them on the first edit. */
export function ApiForm({ error, onClearError, ...props }: Props) {
  return (
    <div className="contents" onChange={error ? onClearError : undefined}>
      {/* display:contents so a dialog's body can scroll above its footer */}
      <Form
        className="contents"
        {...props}
        validationErrors={isApiError(error, 422) ? error.fields : undefined}
      />
    </div>
  );
}
