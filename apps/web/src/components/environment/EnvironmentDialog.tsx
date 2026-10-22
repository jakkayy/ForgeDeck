import {
  Alert,
  Button,
  Checkbox,
  CheckboxGroup,
  Description,
  Disclosure,
  FieldError,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  Switch,
  TextArea,
  TextField,
  toast,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { isApiError } from "../../api/client";
import {
  createEnvironment,
  type Environment,
  environmentQuery,
  environmentsQuery,
  type HealthCheck,
  type ServiceConfig,
  updateEnvironment,
} from "../../api/environments";
import { secretsQuery } from "../../api/secrets";
import { serversQuery } from "../../api/servers";
import {
  envToText,
  parseEnv,
  parseOptionalNumber,
  parseVolumes,
  volumesToText,
} from "../../lib/serviceConfig";
import { ApiForm } from "../ApiForm";

type Props = {
  applicationId: string;
  projectId: string;
  environment?: Environment;
};

const healthTypes = [
  { id: "auto", label: "Automatic" },
  { id: "tcp", label: "TCP connect" },
  { id: "http", label: "HTTP request" },
  { id: "docker", label: "Image HEALTHCHECK" },
] as const;

export function EnvironmentDialog({ applicationId, projectId, environment }: Props) {
  const queryClient = useQueryClient();
  const save = useMutation({
    mutationFn: (input: Parameters<typeof createEnvironment>[1]) =>
      environment
        ? updateEnvironment(environment.id, input)
        : createEnvironment(applicationId, input),
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: environmentsQuery(applicationId).queryKey });
      queryClient.invalidateQueries({ queryKey: environmentQuery(saved.id).queryKey });
      toast.success(environment ? `${saved.name} saved` : `${saved.name} created`);
    },
  });

  return (
    <Modal onOpenChange={(open) => !open && save.reset()}>
      <Button size={environment ? "sm" : "md"} variant={environment ? "secondary" : "primary"}>
        {environment ? "Edit" : "New environment"}
      </Button>
      <Modal.Backdrop>
        <Modal.Container size="md" scroll="inside">
          <Modal.Dialog>
            {({ close }) => (
              <EnvironmentForm
                projectId={projectId}
                environment={environment}
                isPending={save.isPending}
                error={save.error}
                onClearError={save.reset}
                onCancel={close}
                onSubmit={(input) => save.mutate(input, { onSuccess: close })}
              />
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}

type FormProps = {
  projectId: string;
  environment?: Environment;
  isPending: boolean;
  error: Error | null;
  onClearError: () => void;
  onCancel: () => void;
  onSubmit: (input: Parameters<typeof createEnvironment>[1]) => void;
};

function EnvironmentForm({
  projectId,
  environment,
  isPending,
  error,
  onClearError,
  onCancel,
  onSubmit,
}: FormProps) {
  const servers = useQuery(serversQuery);
  const secrets = useQuery(secretsQuery(projectId));
  const projectServers = (servers.data?.items ?? []).filter((s) => s.project_id === projectId);
  const config = environment?.config;
  const [serverId, setServerId] = useState<string | null>(environment?.server_id ?? null);
  const [autoDeploy, setAutoDeploy] = useState(environment?.auto_deploy ?? false);
  const [bindAddress, setBindAddress] = useState(config?.bind_address ?? "127.0.0.1");
  const [restart, setRestart] = useState(config?.restart ?? "unless-stopped");
  const [healthType, setHealthType] = useState<string>(config?.health_check.type ?? "auto");
  const [secretNames, setSecretNames] = useState<string[]>(config?.secrets ?? []);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const text = (name: string) => String(data.get(name) ?? "");

    const health: Partial<HealthCheck> = {};
    if (healthType !== "auto") {
      health.type = healthType as HealthCheck["type"];
    }
    if (healthType === "http") {
      health.path = text("health_path").trim();
    }
    const timeout = parseOptionalNumber(text("health_timeout"));
    if (timeout !== null) {
      health.timeout_seconds = timeout;
    }
    onSubmit({
      name: text("name").trim(),
      server_id: serverId ?? "",
      auto_deploy: autoDeploy,
      config: {
        // Editing replaces the whole configuration, so keep what this form cannot show.
        ...(config ?? {}),
        container_port: parseOptionalNumber(text("container_port")),
        published_port: parseOptionalNumber(text("published_port")),
        bind_address: bindAddress as ServiceConfig["bind_address"],
        env: parseEnv(text("env")),
        secrets: secretNames,
        volumes: parseVolumes(text("volumes")),
        restart: restart as ServiceConfig["restart"],
        memory_mb: parseOptionalNumber(text("memory_mb")),
        health_check: health as HealthCheck,
      },
    });
  };

  const noServers = servers.isSuccess && projectServers.length === 0;
  return (
    <ApiForm onSubmit={submit} error={error} onClearError={onClearError}>
      <Modal.Header>
        <Modal.Heading>
          {environment ? `Edit ${environment.name}` : "New environment"}
        </Modal.Heading>
      </Modal.Header>
      <Modal.Body className="flex flex-col gap-4">
        {noServers ? (
          <Alert status="warning">
            <Alert.Content>
              <Alert.Description>
                This project has no server yet. Add one on the Servers page, then come back.
              </Alert.Description>
            </Alert.Content>
          </Alert>
        ) : (
          <>
            <TextField name="name" defaultValue={environment?.name} isRequired autoFocus>
              <Label>Name</Label>
              <Input placeholder="production" />
              <Description>Lowercase letters, digits and dashes.</Description>
              <FieldError />
            </TextField>
            <Select
              name="server_id"
              placeholder="Choose a server"
              isRequired
              selectedKey={serverId}
              onSelectionChange={(key) => setServerId(String(key))}
            >
              <Label>Server</Label>
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {projectServers.map((server) => (
                    <ListBox.Item key={server.id} id={server.id} textValue={server.name}>
                      {server.name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
              <FieldError />
            </Select>
            <div className="grid grid-cols-2 gap-4">
              <TextField
                name="container_port"
                type="number"
                defaultValue={config?.container_port?.toString() ?? ""}
              >
                <Label>Container port</Label>
                <Input placeholder="8080" />
                <FieldError />
              </TextField>
              <TextField
                name="published_port"
                type="number"
                defaultValue={config?.published_port?.toString() ?? ""}
              >
                <Label>Host port</Label>
                <Input placeholder="18080" />
                <FieldError />
              </TextField>
            </div>
            <Switch isSelected={autoDeploy} onChange={setAutoDeploy}>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Switch.Content>
                <Label>Deploy every successful build</Label>
                <Description>
                  Off by default, so this environment changes only when someone deploys.
                </Description>
              </Switch.Content>
            </Switch>
            <Disclosure>
              <Disclosure.Heading>
                <Disclosure.Trigger className="flex items-center gap-1 text-sm">
                  Advanced
                  <Disclosure.Indicator />
                </Disclosure.Trigger>
              </Disclosure.Heading>
              <Disclosure.Content>
                <Disclosure.Body className="flex flex-col gap-4 pt-3">
                  <Select
                    name="bind_address"
                    selectedKey={bindAddress}
                    onSelectionChange={(key) => setBindAddress(String(key) as typeof bindAddress)}
                  >
                    <Label>Listen on</Label>
                    <Select.Trigger>
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        <ListBox.Item id="127.0.0.1" textValue="This server only">
                          This server only (127.0.0.1)
                          <ListBox.ItemIndicator />
                        </ListBox.Item>
                        <ListBox.Item id="0.0.0.0" textValue="All interfaces">
                          All interfaces (0.0.0.0)
                          <ListBox.ItemIndicator />
                        </ListBox.Item>
                      </ListBox>
                    </Select.Popover>
                    <Description>
                      Use this server only behind a reverse proxy on the same host.
                    </Description>
                  </Select>
                  <Select
                    name="health_type"
                    selectedKey={healthType}
                    onSelectionChange={(key) => setHealthType(String(key))}
                  >
                    <Label>Health check</Label>
                    <Select.Trigger>
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {healthTypes.map((t) => (
                          <ListBox.Item key={t.id} id={t.id} textValue={t.label}>
                            {t.label}
                            <ListBox.ItemIndicator />
                          </ListBox.Item>
                        ))}
                      </ListBox>
                    </Select.Popover>
                    <Description>
                      A deployment counts as healthy only when this passes twice in a row.
                    </Description>
                    <FieldError />
                  </Select>
                  {healthType === "http" && (
                    <TextField
                      name="health_path"
                      defaultValue={config?.health_check.path ?? "/healthz"}
                      isRequired
                    >
                      <Label>Health check path</Label>
                      <Input />
                      <FieldError />
                    </TextField>
                  )}
                  <TextField
                    name="health_timeout"
                    type="number"
                    defaultValue={config?.health_check.timeout_seconds?.toString() ?? ""}
                  >
                    <Label>Health check timeout (seconds)</Label>
                    <Input placeholder="60" />
                    <FieldError />
                  </TextField>
                  <TextField name="env" defaultValue={envToText(config?.env ?? {})}>
                    <Label>Environment variables</Label>
                    <TextArea rows={4} placeholder={"LOG_LEVEL=info\nPORT=8080"} />
                    <Description>
                      One NAME=value per line. Not for secrets: they are shown to project members.
                    </Description>
                    <FieldError />
                  </TextField>
                  <CheckboxGroup value={secretNames} onChange={setSecretNames} name="secrets">
                    <Label>Secrets</Label>
                    {secrets.isSuccess && secrets.data.length === 0 && (
                      <Description>
                        No secrets in this project yet. Add them in the project's Secrets tab.
                      </Description>
                    )}
                    {/* A listed secret that was deleted still shows, so it can be unticked. */}
                    {[...new Set([...(secrets.data ?? []).map((s) => s.name), ...secretNames])].map(
                      (name) => (
                        <Checkbox key={name} value={name}>
                          <Checkbox.Control>
                            <Checkbox.Indicator />
                          </Checkbox.Control>
                          <Checkbox.Content>
                            <Label className="font-mono">{name}</Label>
                          </Checkbox.Content>
                        </Checkbox>
                      ),
                    )}
                    <Description>
                      The service receives each ticked secret as an environment variable of the same
                      name at its next deployment.
                    </Description>
                    <FieldError />
                  </CheckboxGroup>
                  <TextField name="volumes" defaultValue={volumesToText(config?.volumes ?? [])}>
                    <Label>Volumes</Label>
                    <TextArea rows={2} placeholder="app-data:/var/lib/app" />
                    <Description>
                      One name:/container/path per line. Named volumes only; they survive
                      deployments.
                    </Description>
                    <FieldError />
                  </TextField>
                  <Select
                    name="restart"
                    selectedKey={restart}
                    onSelectionChange={(key) => setRestart(String(key) as typeof restart)}
                  >
                    <Label>Restart policy</Label>
                    <Select.Trigger>
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {["unless-stopped", "always", "on-failure", "no"].map((policy) => (
                          <ListBox.Item key={policy} id={policy} textValue={policy}>
                            {policy}
                            <ListBox.ItemIndicator />
                          </ListBox.Item>
                        ))}
                      </ListBox>
                    </Select.Popover>
                  </Select>
                  <TextField
                    name="memory_mb"
                    type="number"
                    defaultValue={config?.memory_mb?.toString() ?? ""}
                  >
                    <Label>Memory limit (MB)</Label>
                    <Input placeholder="No limit" />
                    <FieldError />
                  </TextField>
                </Disclosure.Body>
              </Disclosure.Content>
            </Disclosure>
          </>
        )}
        {error && !isApiError(error, 422) && (
          <Alert status="danger">
            <Alert.Content>
              <Alert.Description>{error.message}</Alert.Description>
            </Alert.Content>
          </Alert>
        )}
      </Modal.Body>
      <Modal.Footer>
        <Button variant="secondary" onPress={onCancel}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" isPending={isPending} isDisabled={noServers}>
          {environment ? "Save" : "Create environment"}
        </Button>
      </Modal.Footer>
    </ApiForm>
  );
}
