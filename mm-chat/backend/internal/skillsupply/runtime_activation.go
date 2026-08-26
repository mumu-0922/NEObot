package skillsupply

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxRunOnlySkillActivations = 2

func matchRunOnlySkillInstallations(
	query string,
	library []Installation,
	already map[string]string,
) []Installation {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	result := make([]Installation, 0, maxRunOnlySkillActivations)
	for _, installation := range library {
		if already[installation.ID] != "" || !skillNameMentioned(query, installation.Name) {
			continue
		}
		result = append(result, installation)
		if len(result) == maxRunOnlySkillActivations {
			return result
		}
	}
	queryTerms := skillLexicalTerms(query)
	type scored struct {
		installation Installation
		score        int
	}
	best := []scored{}
	for _, installation := range library {
		if already[installation.ID] != "" || containsInstallation(result, installation.ID) {
			continue
		}
		nameTerms := skillLexicalTerms(installation.Name)
		candidateTerms := skillLexicalTerms(installation.Name + " " + installation.Description)
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
			if utf8.RuneCountInString(term) >= 4 {
				longSharedTerm = true
			}
		}
		if score < 2 && !(score == 1 && longSharedTerm) {
			continue
		}
		best = append(best, scored{installation: installation, score: score})
	}
	for len(result) < maxRunOnlySkillActivations && len(best) > 0 {
		bestIndex := 0
		for index := 1; index < len(best); index++ {
			if best[index].score > best[bestIndex].score ||
				(best[index].score == best[bestIndex].score &&
					best[index].installation.Name < best[bestIndex].installation.Name) {
				bestIndex = index
			}
		}
		result = append(result, best[bestIndex].installation)
		best = append(best[:bestIndex], best[bestIndex+1:]...)
	}
	return result
}

func containsInstallation(items []Installation, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func skillNameMentioned(value string, name string) bool {
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

func skillLexicalTerms(value string) map[string]struct{} {
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
		runes := []rune(term)
		if containsHan(runes) {
			for index := 0; index+1 < len(runes); index++ {
				add(string(runes[index : index+2]))
			}
		}
	}
	return terms
}

func containsHan(value []rune) bool {
	for _, item := range value {
		if unicode.Is(unicode.Han, item) {
			return true
		}
	}
	return false
}
