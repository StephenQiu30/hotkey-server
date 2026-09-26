import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";

type KeywordGroupFieldProps = {
  description: string;
  error?: string;
  disabled?: boolean;
  id: string;
  label: string;
  onChange: (value: string) => void;
  value: string;
};

export function parseKeywordLines(value: string): string[] {
  return value
    .split(/\r?\n/u)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function KeywordGroupField({
  description,
  error,
  disabled = false,
  id,
  label,
  onChange,
  value,
}: KeywordGroupFieldProps) {
  const count = parseKeywordLines(value).length;

  return (
    <Field data-disabled={disabled} data-invalid={Boolean(error)}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Textarea
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        rows={4}
        maxLength={5_000}
        placeholder="每行一个关键词"
        aria-describedby={`${id}-description`}
        aria-invalid={Boolean(error)}
      />
      <FieldDescription id={`${id}-description`}>
        {description} 当前 {count}/50 个关键词。
      </FieldDescription>
      {error ? <FieldError>{error}</FieldError> : null}
    </Field>
  );
}
