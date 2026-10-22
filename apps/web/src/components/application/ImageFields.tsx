import { Description, FieldError, Input, Label, ListBox, Select, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { registriesQuery } from "../../api/registries";

export type ImageValue = { registryId: string | null; repository: string };

type Props = {
  projectId: string;
  value: ImageValue;
  onChange: (value: ImageValue) => void;
};

export function ImageFields({ projectId, value, onChange }: Props) {
  const registries = useQuery(registriesQuery(projectId));
  const host = registries.data?.find((r) => r.id === value.registryId)?.host;

  return (
    <>
      <Select
        placeholder={
          registries.isSuccess && registries.data.length === 0
            ? "Add a registry in the Registries tab"
            : "Choose a registry"
        }
        isDisabled={!registries.isSuccess || registries.data.length === 0}
        selectedKey={value.registryId}
        onSelectionChange={(key) => onChange({ ...value, registryId: key ? String(key) : null })}
      >
        <Label>Registry</Label>
        <Select.Trigger>
          <Select.Value />
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox>
            {(registries.data ?? []).map((registry) => (
              <ListBox.Item key={registry.id} id={registry.id} textValue={registry.name}>
                {registry.name} ({registry.host})
                <ListBox.ItemIndicator />
              </ListBox.Item>
            ))}
          </ListBox>
        </Select.Popover>
      </Select>
      <TextField
        name="image_repository"
        value={value.repository}
        onChange={(repository) => onChange({ ...value, repository })}
        isDisabled={value.registryId === null}
        isRequired={value.registryId !== null}
      >
        <Label>Image</Label>
        <Input placeholder="acme/shop-api" />
        <Description>
          {host
            ? `Builds push to ${host}/${value.repository || "…"}:<commit>.`
            : "Lowercase path without host or tag."}
        </Description>
        <FieldError />
      </TextField>
    </>
  );
}
