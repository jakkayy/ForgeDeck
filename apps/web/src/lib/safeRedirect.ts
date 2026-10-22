/** Returns redirect only if it is a path inside this app, so ?redirect= cannot leave the site. */
export function safeRedirect(redirect: unknown): string | undefined {
  if (typeof redirect !== "string" || !redirect.startsWith("/") || redirect.startsWith("//")) {
    return undefined;
  }
  return redirect;
}
