import { Button } from "@heroui/react";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, redirect, useNavigate } from "@tanstack/react-router";
import { logout, meQuery } from "../api/auth";

/** Layout for every signed-in page; redirects to /login without a session. */
export const Route = createFileRoute("/_app")({
  beforeLoad: async ({ context, location }) => {
    const user = await context.queryClient.ensureQueryData(meQuery).catch(() => undefined);
    if (!user) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: AppShell,
});

const navItems = [
  { to: "/", label: "Overview" },
  { to: "/servers", label: "Servers" },
  { to: "/projects", label: "Projects" },
  { to: "/jobs", label: "Jobs" },
] as const;

function AppShell() {
  return (
    <div className="min-h-dvh">
      <header className="border-b border-[var(--separator)] bg-[var(--surface)]">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
          <Link to="/" className="font-semibold tracking-tight">
            ForgeDeck
          </Link>
          <nav aria-label="Main" className="flex flex-1 gap-1">
            {navItems.map((item) => (
              <Link
                key={item.to}
                to={item.to}
                className="rounded-md px-3 py-1.5 text-sm text-[var(--muted)] hover:text-[var(--foreground)]"
                activeProps={{
                  className: "bg-[var(--surface-secondary)] !text-[var(--foreground)]",
                }}
              >
                {item.label}
              </Link>
            ))}
          </nav>
          <UserMenu />
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}

function UserMenu() {
  const { data: user } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const signOut = useMutation({
    mutationFn: logout,
    onSettled: () => {
      queryClient.clear();
      navigate({ to: "/login", search: { redirect: undefined } });
    },
  });

  return (
    <div className="flex items-center gap-3 text-sm">
      <span className="hidden text-[var(--muted)] sm:inline">{user.email}</span>
      <Button
        size="sm"
        variant="ghost"
        onPress={() => signOut.mutate()}
        isPending={signOut.isPending}
      >
        Sign out
      </Button>
    </div>
  );
}
