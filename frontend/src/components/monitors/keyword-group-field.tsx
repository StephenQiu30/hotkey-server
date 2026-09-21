import { Label } from "../ui/label";
import { Textarea } from "../ui/textarea";

type KeywordGroupFieldProps = {
  description: string;
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
  disabled = false,
  id,
  label,
  onChange,
  value,
}: KeywordGroupFieldProps) {
  const count = parseKeywordLines(value).length;

  return (
    <div className="space-y-2">
      <div className="flex items-end justify-between gap-4">
        <Label htmlFor={id}>{label}</Label>
        <span className="text-muted-foreground text-xs">{count}/50</span>
      </div>
      <Textarea
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        rows={4}
        maxLength={5_000}
        placeholder="每行一个关键词"
        aria-describedby={`${id}-description`}
      />
      <p
        id={`${id}-description`}
        className="text-muted-foreground text-xs leading-5"
      >
        {description}
      </p>
    </div>
  );
}
