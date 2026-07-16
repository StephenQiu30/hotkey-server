package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	canonicaldb "github.com/StephenQiu30/hotkey-server/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaInitLock = "hotkey-schema-init-v1"

var createTablePattern = regexp.MustCompile(`(?im)^\s*CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-z_][a-z0-9_]*)\s*\(`)
var createIndexPattern = regexp.MustCompile(`(?im)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+([a-z_][a-z0-9_]*)\s+ON\s+`)
var alterTableAddForeignKeyPattern = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+([a-z_][a-z0-9_]*)\s+ADD\s+CONSTRAINT\s+([a-z_][a-z0-9_]*)\s+(FOREIGN\s+KEY\s*\([^)]+\)\s+REFERENCES\s+[a-z_][a-z0-9_]*\s*\([^)]+\)\s+ON\s+DELETE\s+[a-z_]+(?:\s+[a-z_]+)?)\s*$`)
var uniqueWordPattern = regexp.MustCompile(`\bunique\b`)
var checkWordPattern = regexp.MustCompile(`\bcheck\b`)
var referencesWordPattern = regexp.MustCompile(`\breferences\b`)
var betweenPattern = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_]*)\s+between\s+([^()\s]+)\s+and\s+([^()\s]+)`)
var castPattern = regexp.MustCompile(`::(?:character varying|timestamp with time zone|double precision|[a-z_]+)(?:\[\])?`)
var spacePattern = regexp.MustCompile(`\s+`)
var quotedNumberPattern = regexp.MustCompile(`'(-?[0-9]+(?:\.[0-9]+)?)'`)

// Verification is a safe compatibility summary that does not expose the DSN
// or arbitrary server configuration.
type Verification struct {
	ServerVersion      int
	CatalogFingerprint string
	Tables             []string
}

// InitializeEmpty applies the canonical embedded schema only to an empty
// public schema. An advisory transaction lock prevents concurrent initializers
// and a failed application rolls back as one unit.
func InitializeEmpty(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("database pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin schema initialization transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", schemaInitLock); err != nil {
		return fmt.Errorf("lock schema initialization: %w", err)
	}
	var publicObjectCount int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM (
    SELECT c.oid
    FROM pg_class c
    JOIN pg_namespace namespace ON namespace.oid = c.relnamespace
    WHERE namespace.nspname = 'public'
      AND c.relkind IN ('r', 'v', 'm', 'S', 'f', 'p', 'c')
    UNION ALL
    SELECT procedure.oid
    FROM pg_proc procedure
    JOIN pg_namespace namespace ON namespace.oid = procedure.pronamespace
    WHERE namespace.nspname = 'public'
    UNION ALL
    SELECT typ.oid
    FROM pg_type typ
    JOIN pg_namespace namespace ON namespace.oid = typ.typnamespace
    WHERE namespace.nspname = 'public'
      AND typ.typrelid = 0
) AS public_objects`,
	).Scan(&publicObjectCount); err != nil {
		return fmt.Errorf("inspect target database: %w", err)
	}
	if publicObjectCount != 0 {
		return fmt.Errorf("refuse db init: public schema is not empty (%d object(s))", publicObjectCount)
	}

	if _, err := tx.Exec(ctx, canonicaldb.SchemaSQL); err != nil {
		return fmt.Errorf("apply embedded schema: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schema initialization: %w", err)
	}
	return nil
}

// Verify performs read-only compatibility checks over the canonical embedded
// schema. It verifies the exact table set, a primary-key constraint for every
// canonical table, required extensions, and returns a deterministic catalog
// fingerprint for operational evidence.
func Verify(ctx context.Context, pool *pgxpool.Pool) (Verification, error) {
	if pool == nil {
		return Verification{}, errors.New("database pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Verification{}, fmt.Errorf("begin read-only compatibility transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var version int
	if err := tx.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		return Verification{}, fmt.Errorf("read PostgreSQL version: %w", err)
	}
	if version < 160000 {
		return Verification{}, fmt.Errorf("PostgreSQL version %d is unsupported: require 16+", version)
	}

	if err := verifyExtensions(ctx, tx); err != nil {
		return Verification{}, err
	}

	expectedContract, err := canonicalCatalogContract()
	if err != nil {
		return Verification{}, fmt.Errorf("derive embedded schema contract: %w", err)
	}
	expected := expectedContract.TableNames()
	actual, err := publicTableNames(ctx, tx)
	if err != nil {
		return Verification{}, err
	}
	if !slices.Equal(expected, actual) {
		return Verification{}, fmt.Errorf("canonical table set mismatch: missing=%v unexpected=%v", difference(expected, actual), difference(actual, expected))
	}

	actualContract, err := databaseCatalogContract(ctx, tx, expected)
	if err != nil {
		return Verification{}, err
	}
	if err := expectedContract.Compare(actualContract); err != nil {
		return Verification{}, fmt.Errorf("canonical catalog contract mismatch: %w", err)
	}
	if expectedContract.Fingerprint() != actualContract.Fingerprint() {
		return Verification{}, errors.New("canonical catalog fingerprint mismatch")
	}

	fingerprint, err := catalogFingerprint(ctx, tx)
	if err != nil {
		return Verification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Verification{}, fmt.Errorf("complete read-only compatibility transaction: %w", err)
	}
	return Verification{ServerVersion: version, CatalogFingerprint: fingerprint, Tables: actual}, nil
}

func verifyExtensions(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, "SELECT extname FROM pg_extension WHERE extname = ANY($1)", []string{"pg_trgm", "vector"})
	if err != nil {
		return fmt.Errorf("read required extensions: %w", err)
	}
	defer rows.Close()

	actual := make([]string, 0, 2)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan required extension: %w", err)
		}
		actual = append(actual, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate required extensions: %w", err)
	}
	slices.Sort(actual)
	expected := []string{"pg_trgm", "vector"}
	if !slices.Equal(expected, actual) {
		return fmt.Errorf("required extensions missing: %v", difference(expected, actual))
	}
	return nil
}

func publicTableNames(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return tableNameQuery(ctx, tx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
}

func tableNameQuery(ctx context.Context, tx pgx.Tx, query string) ([]string, error) {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read catalog table names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan catalog table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog table names: %w", err)
	}
	return names, nil
}

func catalogFingerprint(ctx context.Context, tx pgx.Tx) (string, error) {
	rows, err := tx.Query(ctx, `
SELECT c.relname, a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod), a.attnotnull,
       COALESCE(pg_get_expr(d.adbin, d.adrelid), '')
FROM pg_class c
JOIN pg_namespace namespace ON namespace.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid
LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
WHERE namespace.nspname = 'public'
  AND c.relkind = 'r'
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY c.relname, a.attnum`)
	if err != nil {
		return "", fmt.Errorf("read catalog fingerprint: %w", err)
	}
	defer rows.Close()

	hash := sha256.New()
	for rows.Next() {
		var table, column, columnType, defaultValue string
		var notNull bool
		if err := rows.Scan(&table, &column, &columnType, &notNull, &defaultValue); err != nil {
			return "", fmt.Errorf("scan catalog fingerprint: %w", err)
		}
		_, _ = fmt.Fprintf(hash, "%s|%s|%s|%t|%s\n", table, column, columnType, notNull, defaultValue)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate catalog fingerprint: %w", err)
	}

	constraints, err := tx.Query(ctx, `
SELECT c.relname, con.conname, con.contype::text,
       pg_catalog.pg_get_constraintdef(con.oid, true)
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace namespace ON namespace.oid = c.relnamespace
WHERE namespace.nspname = 'public'
ORDER BY c.relname, con.contype, con.conname`)
	if err != nil {
		return "", fmt.Errorf("read catalog constraints: %w", err)
	}
	defer constraints.Close()
	for constraints.Next() {
		var table, name, kind, definition string
		if err := constraints.Scan(&table, &name, &kind, &definition); err != nil {
			return "", fmt.Errorf("scan catalog constraint: %w", err)
		}
		_, _ = fmt.Fprintf(hash, "constraint|%s|%s|%s|%s\n", table, name, kind, definition)
	}
	if err := constraints.Err(); err != nil {
		return "", fmt.Errorf("iterate catalog constraints: %w", err)
	}

	indexes, err := tx.Query(ctx, `
SELECT tablename, indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public'
ORDER BY tablename, indexname`)
	if err != nil {
		return "", fmt.Errorf("read catalog indexes: %w", err)
	}
	defer indexes.Close()
	for indexes.Next() {
		var table, name, definition string
		if err := indexes.Scan(&table, &name, &definition); err != nil {
			return "", fmt.Errorf("scan catalog index: %w", err)
		}
		_, _ = fmt.Fprintf(hash, "index|%s|%s|%s\n", table, name, definition)
	}
	if err := indexes.Err(); err != nil {
		return "", fmt.Errorf("iterate catalog indexes: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type catalogContract struct {
	Tables  map[string]tableCatalogContract
	Indexes []string
}

type tableCatalogContract struct {
	Columns     []columnCatalogContract
	Constraints constraintCounts
	Definitions []string
}

type columnCatalogContract struct {
	Name    string
	Type    string
	NotNull bool
	Default string
}

type constraintCounts struct {
	Primary int
	Unique  int
	Foreign int
	Check   int
}

func canonicalCatalogContract() (catalogContract, error) {
	contract := catalogContract{Tables: make(map[string]tableCatalogContract)}
	for _, statement := range strings.Split(canonicaldb.SchemaSQL, ";") {
		if indexMatch := createIndexPattern.FindStringSubmatchIndex(statement); indexMatch != nil {
			name := statement[indexMatch[2]:indexMatch[3]]
			contract.Indexes = append(contract.Indexes, indexSignature(name, statement))
		}
		if alterMatch := alterTableAddForeignKeyPattern.FindStringSubmatch(statement); alterMatch != nil {
			table := alterMatch[1]
			existing, found := contract.Tables[table]
			if !found {
				return catalogContract{}, fmt.Errorf("altered table %s is not defined before its constraint", table)
			}
			_, constraints, definitions, err := parseTableDefinition(alterMatch[3])
			if err != nil {
				return catalogContract{}, fmt.Errorf("parse altered table %s constraint %s: %w", table, alterMatch[2], err)
			}
			existing.Constraints.Primary += constraints.Primary
			existing.Constraints.Unique += constraints.Unique
			existing.Constraints.Foreign += constraints.Foreign
			existing.Constraints.Check += constraints.Check
			existing.Definitions = append(existing.Definitions, definitions...)
			slices.Sort(existing.Definitions)
			contract.Tables[table] = existing
		}
		match := createTablePattern.FindStringSubmatchIndex(statement)
		if match == nil {
			continue
		}
		table := statement[match[2]:match[3]]
		open := strings.Index(statement[match[0]:match[1]], "(")
		if open < 0 {
			return catalogContract{}, fmt.Errorf("find opening parenthesis for %s", table)
		}
		open += match[0]
		close := strings.LastIndex(statement, ")")
		if close <= open {
			return catalogContract{}, fmt.Errorf("find closing parenthesis for %s", table)
		}
		columns, constraints, definitions, err := parseTableDefinition(statement[open+1 : close])
		if err != nil {
			return catalogContract{}, fmt.Errorf("parse table %s: %w", table, err)
		}
		contract.Tables[table] = tableCatalogContract{Columns: columns, Constraints: constraints, Definitions: definitions}
	}
	if len(contract.Tables) == 0 {
		return catalogContract{}, errors.New("embedded schema has no tables")
	}
	slices.Sort(contract.Indexes)
	return contract, nil
}

func parseTableDefinition(definition string) ([]columnCatalogContract, constraintCounts, []string, error) {
	parts, err := splitDefinitionParts(definition)
	if err != nil {
		return nil, constraintCounts{}, nil, err
	}
	columns := make([]columnCatalogContract, 0, len(parts))
	constraints := constraintCounts{}
	definitions := make([]string, 0)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if !isTableConstraint(lower) {
			fields := strings.Fields(part)
			if len(fields) < 2 {
				return nil, constraintCounts{}, nil, fmt.Errorf("column definition %q has no type", part)
			}
			columns = append(columns, columnCatalogContract{
				Name:    fields[0],
				Type:    normalizeSQLType(fields[1]),
				NotNull: strings.Contains(lower, "not null") || strings.Contains(lower, "primary key"),
				Default: normalizeDefault(extractDefault(part)),
			})
		}
		constraints.add(lower)
		definitions = append(definitions, expectedConstraintDefinitions(part)...)
	}
	slices.Sort(definitions)
	return columns, constraints, definitions, nil
}

func splitDefinitionParts(definition string) ([]string, error) {
	parts := make([]string, 0)
	start, depth := 0, 0
	inQuote := false
	for index, char := range definition {
		switch char {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
				if depth < 0 {
					return nil, errors.New("unbalanced parentheses")
				}
			}
		case ',':
			if !inQuote && depth == 0 {
				parts = append(parts, definition[start:index])
				start = index + 1
			}
		}
	}
	if inQuote || depth != 0 {
		return nil, errors.New("unbalanced table definition")
	}
	parts = append(parts, definition[start:])
	return parts, nil
}

func isTableConstraint(definition string) bool {
	return strings.HasPrefix(definition, "constraint ") ||
		strings.HasPrefix(definition, "primary key") ||
		strings.HasPrefix(definition, "unique ") ||
		strings.HasPrefix(definition, "foreign key") ||
		strings.HasPrefix(definition, "check")
}

func (counts *constraintCounts) add(definition string) {
	if strings.Contains(definition, "primary key") {
		counts.Primary++
	}
	if uniqueWordPattern.MatchString(definition) {
		counts.Unique++
	}
	if referencesWordPattern.MatchString(definition) {
		counts.Foreign++
	}
	if checkWordPattern.MatchString(definition) {
		counts.Check++
	}
}

func normalizeSQLType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(value, "varchar"):
		return "character varying" + strings.TrimPrefix(value, "varchar")
	case strings.HasPrefix(value, "char("):
		return "character" + strings.TrimPrefix(value, "char")
	case value == "timestamptz":
		return "timestamp with time zone"
	default:
		return value
	}
}

func extractDefault(definition string) string {
	lower := strings.ToLower(definition)
	start := 0
	for {
		next := strings.Index(lower[start:], " default ")
		if next < 0 {
			return ""
		}
		start += next
		if !strings.HasPrefix(strings.TrimSpace(lower[start+len(" default "):]), "as identity") {
			break
		}
		start += len(" default ")
	}
	value := strings.TrimSpace(definition[start+len(" default "):])
	for _, marker := range []string{" not null", " unique", " references", " check", " primary key"} {
		if end := strings.Index(strings.ToLower(value), marker); end >= 0 {
			value = strings.TrimSpace(value[:end])
		}
	}
	return value
}

func expectedConstraintDefinitions(part string) []string {
	lower := strings.ToLower(strings.TrimSpace(part))
	if lower == "" {
		return nil
	}
	if isTableConstraint(lower) {
		kind := ""
		switch {
		case strings.HasPrefix(lower, "primary key"):
			kind = "p"
		case strings.HasPrefix(lower, "unique"):
			kind = "u"
		case strings.HasPrefix(lower, "foreign key"):
			kind = "f"
		case strings.HasPrefix(lower, "check"):
			kind = "c"
		case strings.HasPrefix(lower, "constraint "):
			for _, candidate := range []struct {
				marker string
				kind   string
			}{{" primary key", "p"}, {" unique", "u"}, {" foreign key", "f"}, {" check", "c"}} {
				if strings.Contains(lower, candidate.marker) {
					kind = candidate.kind
					break
				}
			}
		}
		if kind == "" {
			return nil
		}
		return []string{constraintSignature(kind, part)}
	}
	fields := strings.Fields(part)
	if len(fields) < 2 {
		return nil
	}
	column := fields[0]
	definitions := make([]string, 0, 4)
	if strings.Contains(lower, "primary key") {
		definitions = append(definitions, constraintSignature("p", "PRIMARY KEY ("+column+")"))
	}
	if uniqueWordPattern.MatchString(lower) {
		definitions = append(definitions, constraintSignature("u", "UNIQUE ("+column+")"))
	}
	if referencesWordPattern.MatchString(lower) {
		start := strings.Index(lower, "references")
		definitions = append(definitions, constraintSignature("f", "FOREIGN KEY ("+column+") "+part[start:]))
	}
	if checkWordPattern.MatchString(lower) {
		start := strings.Index(lower, "check")
		definitions = append(definitions, constraintSignature("c", part[start:]))
	}
	return definitions
}

func constraintSignature(kind, definition string) string {
	return kind + "|" + normalizeCatalogExpression(definition)
}

func indexSignature(name, definition string) string {
	return name + "|" + normalizeIndexDefinition(definition)
}

func normalizeDefault(value string) string {
	return normalizeCatalogExpression(value)
}

func normalizeIndexDefinition(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "if not exists", "")
	value = strings.ReplaceAll(value, "public.", "")
	value = strings.ReplaceAll(value, " using btree", "")
	if where := strings.Index(value, " where "); where >= 0 {
		predicate := strings.TrimSpace(value[where+len(" where "):])
		value = value[:where+len(" where ")] + stripOuterParentheses(predicate)
	}
	return normalizeCatalogExpression(value)
}

func normalizeCatalogExpression(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, `"`, "")
	value = betweenPattern.ReplaceAllString(value, "$1 >= $2 and $1 <= $3")
	value = castPattern.ReplaceAllString(value, "")
	value = quotedNumberPattern.ReplaceAllString(value, "$1")
	value = spacePattern.ReplaceAllString(value, "")
	value = normalizeInExpression(value)
	value = stripRedundantParentheses(value)
	return value
}

func normalizeInExpression(value string) string {
	for {
		start := strings.Index(value, "in(")
		if start < 1 {
			return value
		}
		identifierStart := start - 1
		for identifierStart >= 0 && (value[identifierStart] == '_' || (value[identifierStart] >= 'a' && value[identifierStart] <= 'z') || (value[identifierStart] >= '0' && value[identifierStart] <= '9')) {
			identifierStart--
		}
		if identifierStart == start-1 {
			return value
		}
		end := matchingParenthesis(value, start+2)
		if end < 0 {
			return value
		}
		identifier := value[identifierStart+1 : start]
		value = value[:identifierStart+1] + identifier + "=any(array[" + value[start+3:end] + "])" + value[end+1:]
	}
}

func stripRedundantParentheses(value string) string {
	for {
		changed := false
		for index := 0; index+2 < len(value); index++ {
			if value[index] != '(' {
				continue
			}
			end := matchingParenthesis(value, index)
			if end < 0 {
				continue
			}
			inner := value[index+1 : end]
			if strings.HasSuffix(value[:index], "and") || strings.HasSuffix(value[:index], "or") {
				value = value[:index] + inner + value[end+1:]
				changed = true
				break
			}
			if isSimpleIdentifier(inner) {
				value = value[:index] + inner + value[end+1:]
				changed = true
				break
			}
			if !strings.Contains(inner, "and") || strings.Contains(inner, "or") {
				continue
			}
			value = value[:index] + inner + value[end+1:]
			changed = true
			break
		}
		if !changed {
			return value
		}
	}
}

func isSimpleIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] != '_' && (value[index] < 'a' || value[index] > 'z') && (value[index] < '0' || value[index] > '9') {
			return false
		}
	}
	return true
}

func matchingParenthesis(value string, start int) int {
	depth := 0
	inQuote := false
	for index := start; index < len(value); index++ {
		switch value[index] {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
				if depth == 0 {
					return index
				}
			}
		}
	}
	return -1
}

func stripOuterParentheses(value string) string {
	value = strings.TrimSpace(value)
	for len(value) >= 2 && value[0] == '(' && matchingParenthesis(value, 0) == len(value)-1 {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

func (contract catalogContract) TableNames() []string {
	names := make([]string, 0, len(contract.Tables))
	for name := range contract.Tables {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (contract catalogContract) Compare(actual catalogContract) error {
	for _, table := range contract.TableNames() {
		expectedTable := contract.Tables[table]
		actualTable, found := actual.Tables[table]
		if !found {
			return fmt.Errorf("table %s is missing", table)
		}
		if !slices.EqualFunc(expectedTable.Columns, actualTable.Columns, func(left, right columnCatalogContract) bool {
			return left == right
		}) {
			return fmt.Errorf("table %s columns differ: expected=%v actual=%v", table, expectedTable.Columns, actualTable.Columns)
		}
		if expectedTable.Constraints != actualTable.Constraints {
			return fmt.Errorf("table %s constraint counts differ: expected=%+v actual=%+v", table, expectedTable.Constraints, actualTable.Constraints)
		}
		if !slices.Equal(expectedTable.Definitions, actualTable.Definitions) {
			return fmt.Errorf("table %s constraint definitions differ: missing=%v unexpected=%v", table, difference(expectedTable.Definitions, actualTable.Definitions), difference(actualTable.Definitions, expectedTable.Definitions))
		}
	}
	if !slices.Equal(contract.Indexes, actual.Indexes) {
		return fmt.Errorf("canonical index set differs: missing=%v unexpected=%v", difference(contract.Indexes, actual.Indexes), difference(actual.Indexes, contract.Indexes))
	}
	return nil
}

func (contract catalogContract) Fingerprint() string {
	hash := sha256.New()
	for _, table := range contract.TableNames() {
		definition := contract.Tables[table]
		for _, column := range definition.Columns {
			_, _ = fmt.Fprintf(hash, "column|%s|%s|%s|%t|%s\n", table, column.Name, column.Type, column.NotNull, column.Default)
		}
		_, _ = fmt.Fprintf(hash, "constraints|%s|%d|%d|%d|%d\n", table, definition.Constraints.Primary, definition.Constraints.Unique, definition.Constraints.Foreign, definition.Constraints.Check)
		for _, constraint := range definition.Definitions {
			_, _ = fmt.Fprintf(hash, "constraint|%s|%s\n", table, constraint)
		}
	}
	for _, index := range contract.Indexes {
		_, _ = fmt.Fprintf(hash, "index|%s\n", index)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func databaseCatalogContract(ctx context.Context, tx pgx.Tx, expectedTables []string) (catalogContract, error) {
	contract := catalogContract{Tables: make(map[string]tableCatalogContract)}
	for _, table := range expectedTables {
		contract.Tables[table] = tableCatalogContract{}
	}
	columns, err := tx.Query(ctx, `
SELECT c.relname, attribute.attname,
       pg_catalog.format_type(attribute.atttypid, attribute.atttypmod),
       attribute.attnotnull,
       COALESCE(pg_catalog.pg_get_expr(default_value.adbin, default_value.adrelid, true), '')
FROM pg_class c
JOIN pg_namespace namespace ON namespace.oid = c.relnamespace
JOIN pg_attribute attribute ON attribute.attrelid = c.oid
LEFT JOIN pg_attrdef default_value ON default_value.adrelid = c.oid AND default_value.adnum = attribute.attnum
WHERE namespace.nspname = 'public'
  AND c.relkind = 'r'
  AND attribute.attnum > 0
  AND NOT attribute.attisdropped
ORDER BY c.relname, attribute.attnum`)
	if err != nil {
		return catalogContract{}, fmt.Errorf("read catalog columns: %w", err)
	}
	defer columns.Close()
	for columns.Next() {
		var table, name, columnType, defaultValue string
		var notNull bool
		if err := columns.Scan(&table, &name, &columnType, &notNull, &defaultValue); err != nil {
			return catalogContract{}, fmt.Errorf("scan catalog column: %w", err)
		}
		definition := contract.Tables[table]
		definition.Columns = append(definition.Columns, columnCatalogContract{Name: name, Type: normalizeSQLType(columnType), NotNull: notNull, Default: normalizeDefault(defaultValue)})
		contract.Tables[table] = definition
	}
	if err := columns.Err(); err != nil {
		return catalogContract{}, fmt.Errorf("iterate catalog columns: %w", err)
	}

	constraints, err := tx.Query(ctx, `
SELECT c.relname, con.contype::text, count(*),
       array_agg(pg_catalog.pg_get_constraintdef(con.oid, true) ORDER BY con.conname)
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace namespace ON namespace.oid = c.relnamespace
WHERE namespace.nspname = 'public'
  AND con.contype IN ('p', 'u', 'f', 'c')
GROUP BY c.relname, con.contype`)
	if err != nil {
		return catalogContract{}, fmt.Errorf("read catalog constraint counts: %w", err)
	}
	defer constraints.Close()
	for constraints.Next() {
		var table, kind string
		var count int
		var definitions []string
		if err := constraints.Scan(&table, &kind, &count, &definitions); err != nil {
			return catalogContract{}, fmt.Errorf("scan catalog constraint count: %w", err)
		}
		definition := contract.Tables[table]
		switch kind {
		case "p":
			definition.Constraints.Primary = count
		case "u":
			definition.Constraints.Unique = count
		case "f":
			definition.Constraints.Foreign = count
		case "c":
			definition.Constraints.Check = count
		}
		for _, constraintDefinition := range definitions {
			definition.Definitions = append(definition.Definitions, constraintSignature(kind, constraintDefinition))
		}
		contract.Tables[table] = definition
	}
	if err := constraints.Err(); err != nil {
		return catalogContract{}, fmt.Errorf("iterate catalog constraint counts: %w", err)
	}
	for table, definition := range contract.Tables {
		slices.Sort(definition.Definitions)
		contract.Tables[table] = definition
	}

	expected, err := canonicalCatalogContract()
	if err != nil {
		return catalogContract{}, err
	}
	expectedIndexNames := make([]string, 0, len(expected.Indexes))
	for _, signature := range expected.Indexes {
		name, _, found := strings.Cut(signature, "|")
		if !found {
			return catalogContract{}, fmt.Errorf("parse canonical index signature %q", signature)
		}
		expectedIndexNames = append(expectedIndexNames, name)
	}
	indexes, err := tx.Query(ctx, `
SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public'
  AND indexname = ANY($1)
ORDER BY indexname`, expectedIndexNames)
	if err != nil {
		return catalogContract{}, fmt.Errorf("read canonical indexes: %w", err)
	}
	defer indexes.Close()
	for indexes.Next() {
		var name, definition string
		if err := indexes.Scan(&name, &definition); err != nil {
			return catalogContract{}, fmt.Errorf("scan canonical index: %w", err)
		}
		contract.Indexes = append(contract.Indexes, indexSignature(name, definition))
	}
	if err := indexes.Err(); err != nil {
		return catalogContract{}, fmt.Errorf("iterate canonical indexes: %w", err)
	}
	return contract, nil
}

func schemaTableNames() []string {
	matches := createTablePattern.FindAllStringSubmatch(canonicaldb.SchemaSQL, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	slices.Sort(names)
	return names
}

func difference(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	difference := make([]string, 0)
	for _, value := range left {
		if _, found := rightSet[value]; !found {
			difference = append(difference, value)
		}
	}
	return difference
}

// EmbeddedSchemaTableNames exposes a copy for tests and database command
// diagnostics without creating another schema manifest.
func EmbeddedSchemaTableNames() []string {
	return append([]string(nil), schemaTableNames()...)
}

// EmbeddedSchemaContains is kept small and explicit for architecture checks.
func EmbeddedSchemaContains(fragment string) bool {
	return strings.Contains(canonicaldb.SchemaSQL, fragment)
}
