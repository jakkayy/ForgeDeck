import { SearchField } from "@heroui/react";

type Props = {
  label: string;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
};

export function SearchBox({ label, placeholder, value, onChange }: Props) {
  return (
    <SearchField aria-label={label} value={value} onChange={onChange} className="w-full sm:w-64">
      <SearchField.Group>
        <SearchField.SearchIcon />
        <SearchField.Input placeholder={placeholder} maxLength={100} />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  );
}
