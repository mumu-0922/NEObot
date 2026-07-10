package migration

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func readPhase15SQL(t *testing.T, path string) string {
	t.Helper()

	contents, err := migrationfiles.FS.ReadFile(path)
	if err != nil {
		t.Fatalf("read embedded Phase 15 migration %s: %v", path, err)
	}

	return normalizePhase15SQL(string(contents))
}

func normalizePhase15SQL(sql string) string {
	sql = regexp.MustCompile(`(?m)--[^\n]*$`).ReplaceAllString(sql, "")
	sql = strings.ToLower(sql)
	sql = strings.NewReplacer(
		`"`, "",
		"`", "",
		"(", " ( ",
		")", " ) ",
		",", " , ",
		";", " ; ",
	).Replace(sql)
	return strings.Join(strings.Fields(sql), " ")
}

func phase15TableBody(sql, table string) (string, bool) {
	pattern := regexp.MustCompile(
		`(?s)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?(?:public\.)?` +
			regexp.QuoteMeta(table) + `\s*\(\s*(.*?)\s*\)\s*;`,
	)
	match := pattern.FindStringSubmatch(sql)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func mustPhase15TableBody(t *testing.T, sql, table string) string {
	t.Helper()

	body, ok := phase15TableBody(sql, table)
	if !ok {
		t.Fatalf("missing CREATE TABLE for Phase 15 invariant table %q", table)
	}
	return body
}

func phase15TableDDL(t *testing.T, sql, table string) string {
	t.Helper()

	body := mustPhase15TableBody(t, sql, table)
	return body + " " + phase15AlterTableDDL(sql, table)
}

func phase15AlterTableDDL(sql, table string) string {
	alterPattern := regexp.MustCompile(
		`(?s)\balter\s+table\s+(?:if\s+exists\s+)?(?:public\.)?` +
			regexp.QuoteMeta(table) + `\b.*?;`,
	)
	return strings.Join(alterPattern.FindAllString(sql, -1), " ")
}

func assertPhase15Columns(t *testing.T, body, table string, columns ...string) {
	t.Helper()

	for _, column := range columns {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(column) + `\b`).MatchString(body) {
			t.Errorf("%s is missing required invariant column %s", table, column)
		}
	}
}

func assertPhase15Fragments(t *testing.T, sql, invariant string, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(sql, fragment) {
			t.Errorf("%s: missing SQL semantic fragment %q", invariant, fragment)
		}
	}
}

func assertPhase15ArrayColumn(t *testing.T, body, column string) {
	t.Helper()

	arrayType := regexp.MustCompile(
		`\b` + regexp.QuoteMeta(column) + `\s+(?:text|varchar)\s*\[\s*\]`,
	)
	jsonArray := regexp.MustCompile(
		`\b` + regexp.QuoteMeta(column) + `\s+jsonb\b.*jsonb_typeof\s*\(\s*` +
			regexp.QuoteMeta(column) + `\s*\)\s*=\s*'array'`,
	)
	if !arrayType.MatchString(body) && !jsonArray.MatchString(body) {
		t.Errorf("processing_consents.%s must have database-enforced array semantics", column)
	}
}

func assertPhase15ReferenceOnDeleteRestrict(
	t *testing.T,
	body string,
	column string,
	targetTable string,
	invariant string,
) {
	t.Helper()

	referencePattern := regexp.MustCompile(
		`\b` + regexp.QuoteMeta(column) + `\b[^,]*\breferences\s+` +
			`(?:public\.)?` + regexp.QuoteMeta(targetTable) +
			`\s*\(\s*id\s*\)\s+on\s+delete\s+restrict`,
	)
	if !referencePattern.MatchString(body) {
		t.Error(invariant)
	}
}

func assertPhase15CompositeForeignKey(
	t *testing.T,
	ddl string,
	targetTable string,
	wantMapping map[string]string,
	invariant string,
) {
	t.Helper()

	foreignKeyPattern := regexp.MustCompile(
		`foreign\s+key\s*\(\s*([^)]*?)\s*\)\s*references\s+` +
			`(?:public\.)?` + regexp.QuoteMeta(targetTable) +
			`\s*\(\s*([^)]*?)\s*\)\s+on\s+delete\s+restrict`,
	)
	for _, match := range foreignKeyPattern.FindAllStringSubmatch(ddl, -1) {
		localColumns := phase15IdentifierList(match[1])
		referencedColumns := phase15IdentifierList(match[2])
		if len(localColumns) != len(referencedColumns) ||
			len(localColumns) != len(wantMapping) {
			continue
		}

		gotMapping := make(map[string]string, len(localColumns))
		for i := range localColumns {
			gotMapping[localColumns[i]] = referencedColumns[i]
		}
		matched := true
		for local, referenced := range wantMapping {
			if gotMapping[local] != referenced {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}

	t.Errorf("%s; the composite FK must use ON DELETE RESTRICT", invariant)
}

func phase15IdentifierList(list string) []string {
	parts := strings.Split(list, ",")
	identifiers := make([]string, 0, len(parts))
	for _, part := range parts {
		identifier := strings.TrimSpace(part)
		if identifier != "" {
			identifiers = append(identifiers, identifier)
		}
	}
	return identifiers
}

func phase15CreatedTables(up string) []string {
	pattern := regexp.MustCompile(
		`\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?(?:public\.)?([a-z_][a-z0-9_]*)\b`,
	)
	seen := make(map[string]struct{})
	for _, match := range pattern.FindAllStringSubmatch(up, -1) {
		seen[match[1]] = struct{}{}
	}

	tables := make([]string, 0, len(seen))
	for table := range seen {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

func phase15CreatedIndexes(up string) []string {
	pattern := regexp.MustCompile(
		`\bcreate\s+(?:unique\s+)?index\s+(?:if\s+not\s+exists\s+)?` +
			`(?:public\.)?([a-z_][a-z0-9_]*)\b`,
	)
	seen := make(map[string]struct{})
	for _, match := range pattern.FindAllStringSubmatch(up, -1) {
		seen[match[1]] = struct{}{}
	}

	indexes := make([]string, 0, len(seen))
	for index := range seen {
		indexes = append(indexes, index)
	}
	sort.Strings(indexes)
	return indexes
}

func phase15DropsTable(down, table string) bool {
	pattern := regexp.MustCompile(
		`(?s)\bdrop\s+table\s+(?:if\s+exists\s+)?[^;]*\b` +
			regexp.QuoteMeta(table) + `\b[^;]*;`,
	)
	return pattern.MatchString(down)
}

func phase15DropsIndex(down, index string) bool {
	pattern := regexp.MustCompile(
		`(?s)\bdrop\s+index\s+(?:if\s+exists\s+)?(?:public\.)?` +
			regexp.QuoteMeta(index) + `\b[^;]*;`,
	)
	return pattern.MatchString(down)
}

func assertPhase15Order(t *testing.T, sql, before, after, reason string) {
	t.Helper()

	beforeOffset := strings.Index(sql, before)
	afterOffset := strings.Index(sql, after)
	if beforeOffset < 0 {
		t.Errorf("Phase 15 rollback is missing %q (%s)", before, reason)
		return
	}
	if afterOffset < 0 {
		t.Errorf("Phase 15 rollback is missing %q (%s)", after, reason)
		return
	}
	if beforeOffset >= afterOffset {
		t.Errorf("Phase 15 rollback must place %q before %q because %s", before, after, reason)
	}
}

func phase15UserAlterColumns(sql, action string) []string {
	alterPattern := regexp.MustCompile(
		`(?s)\balter\s+table\s+(?:if\s+exists\s+)?(?:public\.)?users\b(.*?);`,
	)
	columnPattern := regexp.MustCompile(
		`\b` + regexp.QuoteMeta(action) +
			`\s+column\s+(?:if\s+(?:not\s+)?exists\s+)?([a-z_][a-z0-9_]*)\b`,
	)
	seen := make(map[string]struct{})
	for _, alter := range alterPattern.FindAllStringSubmatch(sql, -1) {
		for _, column := range columnPattern.FindAllStringSubmatch(alter[1], -1) {
			seen[column[1]] = struct{}{}
		}
	}

	columns := make([]string, 0, len(seen))
	for column := range seen {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func containsPhase15String(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
