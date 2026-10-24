import {
  Button,
  Card,
  Description,
  FieldError,
  Input,
  Label,
  Modal,
  TextField,
} from "@heroui/react";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import { isApiError } from "../../../api/client";
import { loadedItems } from "../../../api/paged";
import { createProject, projectListQuery, projectsQuery } from "../../../api/projects";
import { ApiForm } from "../../../components/ApiForm";
import { EmptyState } from "../../../components/EmptyState";
import { LoadMore } from "../../../components/LoadMore";
import { PageHeader } from "../../../components/PageHeader";
import { QueryBoundary } from "../../../components/QueryBoundary";
import { RoleChip } from "../../../components/RoleChip";
import { SearchBox } from "../../../components/SearchBox";
import { useDebouncedValue } from "../../../lib/useDebouncedValue";

export const Route = createFileRoute("/_app/projects/")({
  loader: ({ context }) => context.queryClient.prefetchInfiniteQuery(projectListQuery()),
  component: ProjectsPage,
});

function ProjectsPage() {
  const [search, setSearch] = useState("");
  const q = useDebouncedValue(search.trim()) || undefined;
  const projects = useInfiniteQuery(projectListQuery(q));
  const noProjectsAtAll =
    !q && projects.data !== undefined && loadedItems(projects.data).length === 0;

  return (
    <>
      <PageHeader
        title="Projects"
        description="Projects group servers, applications and the people who can manage them."
        actions={noProjectsAtAll ? undefined : <NewProjectDialog />}
      />
      {!noProjectsAtAll && (
        <div className="mb-4">
          <SearchBox
            label="Search projects"
            placeholder="Search by name"
            value={search}
            onChange={setSearch}
          />
        </div>
      )}
      <QueryBoundary
        query={projects}
        label="projects"
        isEmpty={(data) => loadedItems(data).length === 0}
        empty={
          q ? (
            <EmptyState title="No projects match" description="Try a different name." />
          ) : (
            <EmptyState
              title="No projects yet"
              description="Create a project first. Servers and applications always belong to a project."
              action={<NewProjectDialog />}
            />
          )
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {loadedItems(data).map((project) => (
                <li key={project.id} className="flex items-center justify-between gap-4 px-4 py-3">
                  <div className="min-w-0">
                    <Link
                      to="/projects/$projectId"
                      params={{ projectId: project.id }}
                      className="block truncate font-medium hover:underline"
                    >
                      {project.name}
                    </Link>
                    <p className="truncate font-mono text-xs text-[var(--muted)]">{project.slug}</p>
                  </div>
                  <RoleChip role={project.role} />
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
      <LoadMore query={projects} />
    </>
  );
}

function NewProjectDialog() {
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: createProject,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectsQuery.queryKey }),
  });

  return (
    <Modal onOpenChange={(open) => !open && create.reset()}>
      <Button variant="primary">New project</Button>
      <Modal.Backdrop>
        <Modal.Container size="sm">
          <Modal.Dialog>
            {({ close }) => {
              const onSubmit = (event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                const data = new FormData(event.currentTarget);
                const slug = String(data.get("slug")).trim();
                create.mutate(
                  { name: String(data.get("name")), slug: slug || undefined },
                  { onSuccess: close },
                );
              };
              return (
                <ApiForm onSubmit={onSubmit} error={create.error} onClearError={create.reset}>
                  <Modal.Header>
                    <Modal.Heading>New project</Modal.Heading>
                  </Modal.Header>
                  <Modal.Body className="flex flex-col gap-4">
                    <TextField name="name" isRequired autoFocus>
                      <Label>Name</Label>
                      <Input placeholder="My Startup" />
                      <FieldError />
                    </TextField>
                    <TextField name="slug">
                      <Label>Slug</Label>
                      <Input placeholder="my-startup" />
                      <Description>Optional. Generated from the name when left empty.</Description>
                      <FieldError />
                    </TextField>
                    {create.error && !isApiError(create.error, 422) && (
                      <p className="text-sm text-[var(--danger)]">{create.error.message}</p>
                    )}
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary" onPress={close}>
                      Cancel
                    </Button>
                    <Button type="submit" variant="primary" isPending={create.isPending}>
                      Create project
                    </Button>
                  </Modal.Footer>
                </ApiForm>
              );
            }}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
