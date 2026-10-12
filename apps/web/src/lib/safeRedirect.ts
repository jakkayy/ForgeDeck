/**
 * Returns redirect only when it is a path inside this app, so a crafted
 * ?redirect= link cannot send a signed-in user to another site.
 */
export function safeRedirect(redirect: unknown): string | undefined {
  if (typeof redirect !== "string" || !redirect.startsWith("/") || redirect.startsWith("//")) {
    return undefined;
  }
  return redirect;
}
