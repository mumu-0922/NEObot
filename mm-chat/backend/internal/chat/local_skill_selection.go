package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/skillsupply"
)

var localSkillGestureNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (runtime *localSkillToolRuntime) promptInstruction() string {
	if !runtime.catalogAvailable() {
		return ""
	}
	items := make([]map[string]string, 0, min(len(runtime.skills), maxLocalSkillPromptItems))
	for index, skill := range runtime.skills {
		if index >= maxLocalSkillPromptItems {
			break
		}
		items = append(items, map[string]string{
			"name":        truncateProcessUTF8(strings.TrimSpace(skill.Name), 128),
			"version":     truncateProcessUTF8(strings.TrimSpace(skill.Version), 128),
			"description": truncateProcessUTF8(strings.TrimSpace(skill.Description), maxLocalSkillDescriptionBytes),
		})
	}
	encoded, _ := json.Marshal(map[string]any{
		"kind":                    "installedSkillCatalogReplacement",
		"untrustedRoutingData":    true,
		"catalogRevision":         runtime.catalogRevision,
		"replacement":             true,
		"tombstone":               len(runtime.skills) == 0,
		"shown":                   len(items),
		"total":                   len(runtime.skills),
		"skills":                  items,
		"legacyCatalogToolsShown": false,
	})
	instruction := localSkillSystemInstruction + "\n" + runtime.terminalPromptInstruction()
	workspaceAlias := ""
	if hostRoot := strings.TrimSpace(runtime.config().WorkspaceHostRoot); hostRoot != "" {
		alias, _ := json.Marshal(map[string]any{
			"kind": "authorizedWorkspaceHostRoot", "hostRoot": hostRoot,
		})
		workspaceAlias = "\nThe configured local workspace is the single project identified by " +
			"<authorized_workspace_alias>. When the user supplies a Linux absolute path or " +
			"WSL UNC path under that root, pass the corresponding workspace-relative path to " +
			"File Tools. Terminal starts at the workspace root; use $PWD or relative paths in " +
			"commands instead of repeating the host path. Paths outside this root are unavailable." +
			"\n<authorized_workspace_alias>" + string(alias) + "</authorized_workspace_alias>"
	}
	if runtime.publishToolAvailable() {
		instruction += "\n" + publishFileSystemInstruction
	}
	return instruction + workspaceAlias + "\n<installed_skill_catalog>" + string(encoded) +
		"</installed_skill_catalog>"
}

// prepareUserPrompt recognizes only the claimed current user text. Slash
// invocations load SKILL.md deterministically; ordinary name/description
// matches queue a required first Tool call so execution cannot precede loading.
func (runtime *localSkillToolRuntime) prepareUserPrompt(base, userText string) (string, error) {
	if !runtime.enabled() {
		return base, nil
	}
	invoked := localSkillInvocationNames(userText)
	injections := make([]map[string]any, 0, len(invoked))
	for _, name := range invoked {
		skill, ok := runtime.byName[name]
		if !ok {
			continue
		}
		body, err := skillsupply.ReadRuntimeSkillFile(skill, "SKILL.md")
		if err != nil || !utf8.Valid(body) {
			return "", skillsupply.ErrRuntimeUnavailable
		}
		runtime.loaded[name] = runtime.catalogRevision
		injections = append(injections, map[string]any{
			"name": name, "catalogRevision": runtime.catalogRevision,
			"content": string(body),
		})
	}
	runtime.required = runtime.matchRequiredSkills(userText)
	if len(injections) == 0 {
		return base, nil
	}
	encoded, _ := json.Marshal(map[string]any{
		"untrustedUserAuthorizedSkillInstructions": true,
		"skills": injections,
	})
	base = strings.TrimSpace(base)
	if base != "" {
		base += "\n\n"
	}
	return base + "<user_authorized_skill_instructions>" + string(encoded) +
		"</user_authorized_skill_instructions>", nil
}

func localSkillCatalogRevision(skills []skillsupply.RuntimeSkill) string {
	entries := make([][]string, 0, len(skills))
	for _, skill := range skills {
		entries = append(entries, []string{
			strings.TrimSpace(skill.Name),
			strings.TrimSpace(skill.Version),
			strings.Join(strings.Fields(skill.Description), " "),
			strings.TrimSpace(skill.PackageFingerprint),
			strings.TrimSpace(skill.ActivationSource),
		})
	}
	encoded, _ := json.Marshal(entries)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func localSkillInvocationNames(value string) []string {
	fields := strings.Fields(value)
	result := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) < 2 || field[0] != '/' {
			continue
		}
		name := field[1:]
		if strings.HasPrefix(name, "skill:") {
			name = strings.TrimPrefix(name, "skill:")
		}
		if !localSkillGestureNamePattern.MatchString(name) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func (runtime *localSkillToolRuntime) matchRequiredSkills(userText string) []string {
	if !runtime.enabled() || strings.TrimSpace(userText) == "" {
		return nil
	}
	required := make([]string, 0, len(runtime.skills))
	for _, skill := range runtime.skills {
		name := strings.TrimSpace(skill.Name)
		if runtime.loaded[name] == runtime.catalogRevision {
			continue
		}
		if localSkillNameMentioned(userText, name) {
			required = append(required, name)
		}
	}
	if len(required) > 0 {
		return required
	}

	queryTerms := localSkillLexicalTerms(userText)
	bestName, bestScore := "", 0
	tied := false
	for _, skill := range runtime.skills {
		name := strings.TrimSpace(skill.Name)
		if runtime.loaded[name] == runtime.catalogRevision {
			continue
		}
		nameTerms := localSkillLexicalTerms(name)
		candidateTerms := localSkillLexicalTerms(name + " " + skill.Description)
		score := 0
		longSharedTerm := false
		for term := range queryTerms {
			if _, exists := candidateTerms[term]; !exists {
				continue
			}
			score++
			if _, nameMatch := nameTerms[term]; nameMatch {
				score++
			}
			if utf8.RuneCountInString(term) >= 5 {
				longSharedTerm = true
			}
		}
		if score < 2 && !(score == 1 && len(runtime.skills) == 1 && longSharedTerm) {
			continue
		}
		switch {
		case score > bestScore:
			bestName, bestScore, tied = name, score, false
		case score == bestScore:
			tied = true
		}
	}
	if bestName == "" || tied {
		return nil
	}
	return []string{bestName}
}

func localSkillNameMentioned(value, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	}) {
		if token == name {
			return true
		}
	}
	return false
}

func localSkillLexicalTerms(value string) map[string]struct{} {
	const maxTerms = 64
	stopwords := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "this": {},
		"that": {}, "use": {}, "using": {}, "create": {}, "make": {}, "agent": {},
		"skill": {}, "tool": {}, "file": {}, "files": {},
	}
	terms := make(map[string]struct{}, maxTerms)
	add := func(term string) {
		if len(terms) >= maxTerms {
			return
		}
		term = strings.ToLower(strings.TrimSpace(term))
		if utf8.RuneCountInString(term) < 2 {
			return
		}
		if _, ignored := stopwords[term]; ignored {
			return
		}
		terms[term] = struct{}{}
	}
	for _, term := range strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		add(term)
	}
	cjk := make([]rune, 0, len(value))
	flushCJK := func() {
		for index := 0; index+1 < len(cjk) && len(terms) < maxTerms; index++ {
			add(string(cjk[index : index+2]))
		}
		cjk = cjk[:0]
	}
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			cjk = append(cjk, r)
			continue
		}
		flushCJK()
	}
	flushCJK()
	return terms
}

func appendLocalSkillSystemInstruction(base string, runtime *localSkillToolRuntime) string {
	instruction := runtime.promptInstruction()
	if instruction == "" {
		return base
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return instruction
	}
	return base + "\n\n" + instruction
}
