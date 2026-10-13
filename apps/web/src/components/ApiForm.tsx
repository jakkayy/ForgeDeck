import { Form } from "@heroui/react";
import type { ComponentProps } from "react";
import { isApiError } from "../api/client";

type Props = Omit<ComponentProps<typeof Form>, "validationErrors"> & {
  /** The last submit's error; 422 field errors are shown on their fields. */
  error: Error | null;
  /** Clears error. Called on the first edit after a failed submit. */
  onClearError: () => void;
};

/**
 * A Form that shows API validation errors on their fields and clears them as
 * soon as the user edits, so the corrected form can be submitted right away
 * instead of being blocked by the stale server error.
 */
export function ApiForm({ error, onClearError, ...props }: Props) {
  return (
    // Field change events bubble here; display:contents keeps layout unchanged.
    <div className="contents" onChange={error ? onClearError : undefined}>
      {/* The form must not be a layout box, so a dialog's body can scroll above its footer. */}
      <Form
        className="contents"
        {...props}
        validationErrors={isApiError(error, 422) ? error.fields : undefined}
      />
    </div>
  );
}
