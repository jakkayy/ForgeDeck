import { Alert, Button, Card, FieldError, Input, Label, TextField } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import { login, meQuery, registerFirstUser, setupQuery } from "../api/auth";
import { isApiError } from "../api/client";
import { ApiForm } from "../components/ApiForm";
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
  const setup = useQuery(setupQuery);
  return setup.data ? <CreateFirstAccount /> : <SignIn />;
}

function SignIn() {
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
      <Card className="w-full max-w-sm gap-4 p-6">
        <Card.Header>
          <h1 className="card__title text-xl font-semibold">Sign in to ForgeDeck</h1>
          <Card.Description>Use the account your administrator created for you.</Card.Description>
        </Card.Header>
        <Card.Content>
          <ApiForm
            className="flex flex-col gap-4"
            onSubmit={onSubmit}
            error={error}
            onClearError={signIn.reset}
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
          </ApiForm>
        </Card.Content>
      </Card>
    </main>
  );
}

/** Shown only while no account exists; the API refuses a second registration. */
function CreateFirstAccount() {
  const { redirect: next } = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [mismatch, setMismatch] = useState(false);
  const register = useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      registerFirstUser(email, password),
    onSuccess: (user) => {
      queryClient.setQueryData(meQuery.queryKey, user);
      queryClient.setQueryData(setupQuery.queryKey, false);
      navigate({ to: next ?? "/" });
    },
    onError: (error) => {
      if (isApiError(error, 409)) {
        queryClient.setQueryData(setupQuery.queryKey, false);
      }
    },
  });

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const password = String(data.get("password"));
    if (password !== String(data.get("confirm"))) {
      setMismatch(true);
      return;
    }
    setMismatch(false);
    register.mutate({ email: String(data.get("email")), password });
  };

  const error = register.error;
  return (
    <main className="grid min-h-dvh place-items-center px-4">
      <Card className="w-full max-w-sm gap-4 p-6">
        <Card.Header>
          <h1 className="card__title text-xl font-semibold">Welcome to ForgeDeck</h1>
          <Card.Description>
            Create the first account. It becomes the owner of this installation, and this page
            closes as soon as it exists.
          </Card.Description>
        </Card.Header>
        <Card.Content>
          <ApiForm
            className="flex flex-col gap-4"
            onSubmit={onSubmit}
            error={error}
            onClearError={register.reset}
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
            <TextField name="password" type="password" autoComplete="new-password" isRequired>
              <Label>Password</Label>
              <Input />
              <FieldError />
            </TextField>
            <TextField
              name="confirm"
              type="password"
              autoComplete="new-password"
              isRequired
              isInvalid={mismatch}
              onChange={() => setMismatch(false)}
            >
              <Label>Repeat password</Label>
              <Input />
              <FieldError>The passwords do not match.</FieldError>
            </TextField>
            <Button type="submit" variant="primary" fullWidth isPending={register.isPending}>
              Create account
            </Button>
          </ApiForm>
        </Card.Content>
      </Card>
    </main>
  );
}
