package domain

import "testing"

func TestIntentDefinitionModelsClausesEntitiesAndExamples(t *testing.T) {
	t.Parallel()

	objective, err := NewIntentObjective("  关注上海人工智能企业收购事件  ")
	if err != nil {
		t.Fatalf("NewIntentObjective(): %v", err)
	}
	clauses := []IntentClause{
		mustIntentClause(t, IntentClauseMust, IntentClauseTerm, "人工智能企业"),
		mustIntentClause(t, IntentClauseShould, IntentClausePhrase, "并购"),
		mustIntentClause(t, IntentClauseMustNot, IntentClauseTerm, "招聘"),
		mustIntentClause(t, IntentClauseMust, IntentClauseAction, "收购"),
		mustIntentClause(t, IntentClauseMustNot, IntentClauseLocation, "北京"),
	}
	entities := []IntentEntity{
		mustIntentEntity(t, "wikidata:Q2283", "微软", []string{"Microsoft", "微软公司"}, "企业实体，不是软件产品"),
	}
	examples := []IntentExample{
		mustIntentExample(t, IntentExamplePositive, "微软宣布收购一家上海人工智能公司"),
		mustIntentExample(t, IntentExampleNegative, "微软在上海发布新的招聘岗位"),
	}

	definition, err := NewIntentDefinition(objective, clauses, entities, examples)
	if err != nil {
		t.Fatalf("NewIntentDefinition(): %v", err)
	}
	if got := definition.Objective().String(); got != "关注上海人工智能企业收购事件" {
		t.Fatalf("objective = %q", got)
	}
	if got := len(definition.Clauses()); got != len(clauses) {
		t.Fatalf("clause count = %d, want %d", got, len(clauses))
	}
	if definition.Fingerprint() == "" {
		t.Fatal("definition fingerprint is empty")
	}

	// Returned collections are defensive copies; callers cannot mutate the
	// immutable definition through an accessor.
	returned := definition.Clauses()
	returned[0] = mustIntentClause(t, IntentClauseMust, IntentClauseTerm, "tampered")
	if definition.Clauses()[0].Value() == "tampered" {
		t.Fatal("definition clauses were mutated through an accessor")
	}
}

func TestIntentDefinitionRejectsContradictoryAndDuplicateClauses(t *testing.T) {
	t.Parallel()

	objective, _ := NewIntentObjective("Track acquisitions")
	_, err := NewIntentDefinition(objective, []IntentClause{
		mustIntentClause(t, IntentClauseMust, IntentClauseTerm, "OpenAI"),
		mustIntentClause(t, IntentClauseMustNot, IntentClauseTerm, " openai "),
	}, nil, nil)
	if err == nil {
		t.Fatal("MUST and MUST_NOT contradiction was accepted")
	}
	_, err = NewIntentDefinition(objective, []IntentClause{
		mustIntentClause(t, IntentClauseShould, IntentClauseAction, "acquisition"),
		mustIntentClause(t, IntentClauseShould, IntentClauseAction, "Acquisition"),
	}, nil, nil)
	if err == nil {
		t.Fatal("duplicate normalized clause was accepted")
	}
	if _, err := NewIntentClause(IntentClauseOperator("unknown"), IntentClauseTerm, "value"); err == nil {
		t.Fatal("unknown clause operator was accepted")
	}
	if _, err := NewIntentClause(IntentClauseMust, IntentClauseField("unknown"), "value"); err == nil {
		t.Fatal("unknown clause field was accepted")
	}
	_, err = NewIntentDefinition(objective, []IntentClause{
		mustIntentClause(t, IntentClauseShould, IntentClauseTerm, "OpenAI"),
		mustIntentClause(t, IntentClauseMustNot, IntentClauseTerm, "openai"),
	}, nil, nil)
	if err == nil {
		t.Fatal("SHOULD and MUST_NOT contradiction was accepted")
	}
}

func TestIntentClauseKeepsOperatorAndFieldOrthogonal(t *testing.T) {
	t.Parallel()

	action := mustIntentClause(t, IntentClauseMust, IntentClauseAction, "acquire")
	excludedLocation := mustIntentClause(t, IntentClauseMustNot, IntentClauseLocation, "Beijing")
	if action.Operator() != IntentClauseMust || action.Field() != IntentClauseAction {
		t.Fatalf("action clause = %s/%s", action.Operator(), action.Field())
	}
	if excludedLocation.Operator() != IntentClauseMustNot || excludedLocation.Field() != IntentClauseLocation {
		t.Fatalf("location clause = %s/%s", excludedLocation.Operator(), excludedLocation.Field())
	}
	objective, _ := NewIntentObjective("Track acquisition outside one city")
	if _, err := NewIntentDefinition(objective, []IntentClause{
		mustIntentClause(t, IntentClauseMust, IntentClauseAction, "Paris"),
		mustIntentClause(t, IntentClauseMustNot, IntentClauseLocation, "Paris"),
	}, nil, nil); err != nil {
		t.Fatalf("same value in different fields was treated as a contradiction: %v", err)
	}
	language := mustIntentClause(t, IntentClauseMust, IntentClauseLanguage, "zh-cn")
	region := mustIntentClause(t, IntentClauseShould, IntentClauseRegion, "cn")
	if language.Value() != "zh-CN" || region.Value() != "CN" {
		t.Fatalf("canonical locale clauses = %q/%q", language.Value(), region.Value())
	}
}

func TestIntentEntityRequiresExplicitDisambiguationForAliasCollision(t *testing.T) {
	t.Parallel()

	objective, _ := NewIntentObjective("Track Apple announcements")
	company := mustIntentEntity(t, "wikidata:Q312", "Apple", []string{"苹果"}, "消费电子公司")
	fruitWithoutNote := mustIntentEntity(t, "wikidata:Q89", "苹果", []string{"apple fruit"}, "")
	if _, err := NewIntentDefinition(objective, nil, []IntentEntity{company, fruitWithoutNote}, nil); err == nil {
		t.Fatal("ambiguous entity labels without notes were accepted")
	}

	fruit := mustIntentEntity(t, "wikidata:Q89", "苹果", []string{"apple fruit"}, "水果实体")
	definition, err := NewIntentDefinition(objective, nil, []IntentEntity{company, fruit}, nil)
	if err != nil {
		t.Fatalf("explicitly disambiguated entities: %v", err)
	}
	if got := len(definition.Entities()); got != 2 {
		t.Fatalf("entity count = %d, want 2", got)
	}
}

func TestIntentExamplesRejectDuplicateOrConflictingLabels(t *testing.T) {
	t.Parallel()

	objective, _ := NewIntentObjective("Track safety incidents")
	positive := mustIntentExample(t, IntentExamplePositive, "Factory fire in Suzhou")
	duplicate := mustIntentExample(t, IntentExamplePositive, " factory fire in suzhou ")
	negative := mustIntentExample(t, IntentExampleNegative, "FACTORY FIRE IN SUZHOU")
	if _, err := NewIntentDefinition(objective, nil, nil, []IntentExample{positive, duplicate}); err == nil {
		t.Fatal("duplicate positive example was accepted")
	}
	if _, err := NewIntentDefinition(objective, nil, nil, []IntentExample{positive, negative}); err == nil {
		t.Fatal("same example with positive and negative labels was accepted")
	}
	if _, err := NewIntentExample(IntentExampleLabel("maybe"), "sample"); err == nil {
		t.Fatal("unknown example label was accepted")
	}
}

func TestIntentFingerprintCommitsToStructureNotOnlyTextSequence(t *testing.T) {
	t.Parallel()

	objective, _ := NewIntentObjective("same objective")
	clauseDefinition, err := NewIntentDefinition(objective, []IntentClause{
		mustIntentClause(t, IntentClauseMust, IntentClauseTerm, "x"),
	}, nil, nil)
	if err != nil {
		t.Fatalf("clause definition: %v", err)
	}
	entityDefinition, err := NewIntentDefinition(objective, nil, []IntentEntity{
		mustIntentEntity(t, "must", "x", nil, ""),
	}, nil)
	if err != nil {
		t.Fatalf("entity definition: %v", err)
	}
	if clauseDefinition.Fingerprint() == entityDefinition.Fingerprint() {
		t.Fatal("structurally different definitions produced the same fingerprint")
	}
}

func mustIntentClause(t *testing.T, operator IntentClauseOperator, field IntentClauseField, value string) IntentClause {
	t.Helper()
	clause, err := NewIntentClause(operator, field, value)
	if err != nil {
		t.Fatalf("NewIntentClause(%s, %s): %v", operator, field, err)
	}
	return clause
}

func mustIntentEntity(t *testing.T, canonicalID, displayName string, aliases []string, ambiguityNote string) IntentEntity {
	t.Helper()
	entity, err := NewIntentEntity(canonicalID, displayName, aliases, ambiguityNote)
	if err != nil {
		t.Fatalf("NewIntentEntity(%s): %v", canonicalID, err)
	}
	return entity
}

func mustIntentExample(t *testing.T, label IntentExampleLabel, text string) IntentExample {
	t.Helper()
	example, err := NewIntentExample(label, text)
	if err != nil {
		t.Fatalf("NewIntentExample(%s): %v", label, err)
	}
	return example
}
