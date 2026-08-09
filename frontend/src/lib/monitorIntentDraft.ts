export type MonitorIntentClauseOperator = "must" | "should" | "must_not";
export type MonitorIntentClauseField =
  | "term"
  | "phrase"
  | "action"
  | "location"
  | "language"
  | "region"
  | "source"
  | "time_window";
export type MonitorIntentExampleLabel = "positive" | "negative";

export type MonitorIntentClauseForm = {
  operator: MonitorIntentClauseOperator;
  field: MonitorIntentClauseField;
  value: string;
};

export type MonitorIntentEntityForm = {
  canonicalId: string;
  displayName: string;
  aliasesText: string;
  ambiguityNote: string;
};

export type MonitorIntentExampleForm = {
  label: MonitorIntentExampleLabel;
  text: string;
};

export type MonitorIntentDraftForm = {
  objective: string;
  clauses: MonitorIntentClauseForm[];
  entities: MonitorIntentEntityForm[];
  examples: MonitorIntentExampleForm[];
};

export const emptyMonitorIntentDraft = (): MonitorIntentDraftForm => ({
  objective: "",
  clauses: [],
  entities: [],
  examples: [],
});

const asClauseOperator = (value: string | undefined): MonitorIntentClauseOperator =>
  value === "should" || value === "must_not" ? value : "must";

const asClauseField = (value: string | undefined): MonitorIntentClauseField => {
  switch (value) {
    case "phrase":
    case "action":
    case "location":
    case "language":
    case "region":
    case "source":
    case "time_window":
      return value;
    default:
      return "term";
  }
};

const asExampleLabel = (value: string | undefined): MonitorIntentExampleLabel =>
  value === "negative" ? "negative" : "positive";

export function monitorIntentDraftFromResponse(
  draft: HotKeyAPI.IntentDraftResponseDTO,
): MonitorIntentDraftForm {
  return {
    objective: draft.objective ?? "",
    clauses: (draft.clauses ?? []).map((clause) => ({
      operator: asClauseOperator(clause.operator),
      field: asClauseField(clause.field),
      value: clause.value ?? "",
    })),
    entities: (draft.entities ?? []).map((entity) => ({
      canonicalId: entity.canonical_id ?? "",
      displayName: entity.display_name ?? "",
      aliasesText: (entity.aliases ?? []).join(", "),
      ambiguityNote: entity.ambiguity_note ?? "",
    })),
    examples: (draft.examples ?? []).map((example) => ({
      label: asExampleLabel(example.label),
      text: example.text ?? "",
    })),
  };
}

function normalizedAliases(value: string): string[] {
  const aliases: string[] = [];
  const seen = new Set<string>();
  for (const candidate of value.split(/[,，\n]/u)) {
    const alias = candidate.trim();
    const key = alias.toLocaleLowerCase();
    if (!alias || seen.has(key)) continue;
    seen.add(key);
    aliases.push(alias);
  }
  return aliases;
}

export function monitorIntentDraftRequest(
  draft: MonitorIntentDraftForm,
  expectedResourceVersion: number,
): HotKeyAPI.ReplaceIntentDraftRequestDTO {
  return {
    expected_resource_version: expectedResourceVersion,
    objective: draft.objective.trim(),
    clauses: draft.clauses.flatMap((clause) => {
      const value = clause.value.trim();
      return value
        ? [{ operator: clause.operator, field: clause.field, value }]
        : [];
    }),
    entities: draft.entities.flatMap((entity) => {
      const canonicalId = entity.canonicalId.trim();
      const displayName = entity.displayName.trim();
      return canonicalId && displayName
        ? [
            {
              canonical_id: canonicalId,
              display_name: displayName,
              aliases: normalizedAliases(entity.aliasesText),
              ambiguity_note: entity.ambiguityNote.trim(),
            },
          ]
        : [];
    }),
    examples: draft.examples.flatMap((example) => {
      const text = example.text.trim();
      return text ? [{ label: example.label, text }] : [];
    }),
  };
}

export function validateMonitorIntentDraft(draft: MonitorIntentDraftForm): string | undefined {
  const objective = draft.objective.trim();
  if (!objective) return "请填写监控目标";
  if ([...objective].length > 2000) return "监控目标不能超过 2000 个字符";
  if (draft.clauses.length > 128) return "意图条件不能超过 128 条";
  if (draft.clauses.some((clause) => !clause.value.trim())) {
    return "请补全或删除空的意图条件";
  }
  if (draft.clauses.some((clause) => [...clause.value.trim()].length > 512)) {
    return "单条意图条件不能超过 512 个字符";
  }
  if (draft.entities.length > 64) return "实体不能超过 64 个";
  if (
    draft.entities.some(
      (entity) => !entity.canonicalId.trim() || !entity.displayName.trim(),
    )
  ) {
    return "请补全或删除缺少身份的实体";
  }
  if (draft.entities.some((entity) => normalizedAliases(entity.aliasesText).length > 32)) {
    return "单个实体的别名不能超过 32 个";
  }
  if (draft.examples.length > 64) return "正反例不能超过 64 条";
  if (draft.examples.some((example) => !example.text.trim())) {
    return "请补全或删除空的正反例";
  }
  if (draft.examples.some((example) => [...example.text.trim()].length > 4000)) {
    return "单条正反例不能超过 4000 个字符";
  }

  const positive = new Set<string>();
  const negative = new Set<string>();
  for (const clause of draft.clauses) {
    const key = `${clause.field}\u0000${clause.value.trim().toLocaleLowerCase()}`;
    if (clause.operator === "must_not") negative.add(key);
    else positive.add(key);
  }
  for (const key of positive) {
    if (negative.has(key)) return "同一字段和值不能同时包含与排除";
  }
  return undefined;
}
