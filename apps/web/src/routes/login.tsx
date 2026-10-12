import { Alert, Button, Card, FieldError, Form, Input, Label, TextField } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import type { FormEvent } from "react";
import { login, meQuery } from "../api/auth";
import { isApiError } from "../api/client";
import { safeRedirect } from "../lib/safeRedirect";

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>) => ({
    redirect: safeRedirect(search.redirect),
  }),
  beforeLoad: async ({ context, search }) => {
    const user = await context.queryClient.ensureQueryData(meQuery).catch(() => undefined);
    if (user) {
      throw redirect({ to: search.redirect ?? "/" });
    }
  },
  component: LoginPage,
});

function LoginPage() {
  const { redirect: next } = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const signIn = useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      login(email, password),
    onSuccess: (user) => {
      queryClient.setQueryData(meQuery.queryKey, user);
      navigate({ to: next ?? "/" });
    },
  });

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    signIn.mutate({ email: String(data.get("email")), password: String(data.get("password")) });
  };

  const error = signIn.error;
  return (
    <main className="grid min-h-dvh place-items-center px-4">
      <Card className="w-full max-w-sm">
        <Card.Header>
          <Card.Title className="text-xl">Sign in to ForgeDeck</Card.Title>
          <Card.Description>Use the account your administrator created for you.</Card.Description>
        </Card.Header>
        <Card.Content>
          <Form
            className="flex flex-col gap-4"
            onSubmit={onSubmit}
            validationErrors={isApiError(error, 422) ? error.fields : undefined}
          >
            {error && !isApiError(error, 422) && (
              <Alert status="danger">
                <Alert.Content>
                  <Alert.Description>{error.message}</Alert.Description>
                </Alert.Content>
              </Alert>
            )}
            <TextField name="email" type="email" autoComplete="username" isRequired autoFocus>
              <Label>Email</Label>
              <Input placeholder="you@example.com" />
              <FieldError />
            </TextField>
            <TextField name="password" type="password" autoComplete="current-password" isRequired>
              <Label>Password</Label>
              <Input />
              <FieldError />
            </TextField>
            <Button type="submit" variant="primary" fullWidth isPending={signIn.isPending}>
              Sign in
            </Button>
          </Form>
        </Card.Content>
      </Card>
    </main>
  );
}
