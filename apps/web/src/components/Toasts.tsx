import { Toast } from "@heroui/react";

/** Renders queued toasts; lazy so it stays out of the first-load bundle. */
export default function Toasts() {
  return <Toast.Provider placement="bottom end" />;
}
