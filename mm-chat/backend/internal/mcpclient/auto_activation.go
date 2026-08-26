package mcpclient

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxRunOnlyMCPActivations = 2

func (s *Service) matchRunOnlyServers(
	ctx context.Context,
	userID string,
	conversationID string,
	selected []SelectionServer,
	query string,
) ([]SelectionServer, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	servers, err := s.ListServers(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	selectedKeys := make(map[string]struct{}, len(selected))
	for _, item := range selected {
		selectedKeys[item.Ref.Key()] = struct{}{}
	}
	type scoredServer struct {
		server Server
		score  int
	}
	queryTerms := mcpActivationTerms(query)
	scored := make([]scoredServer, 0, len(servers))
	for _, server := range servers {
		if _, exists := selectedKeys[server.Ref.Key()]; exists ||
			server.Status != ServerStatusReady ||
			(server.AuthType != AuthNone && !server.HasCredential) {
			continue
		}
		score := 0
		if mcpNameMentioned(query, server.Name) {
			score += 8
		}
		nameTerms := mcpActivationTerms(server.Name)
		haystack := server.Name + " " + server.Description
		for _, tool := range server.Tools {
			haystack += " " + tool.Name + " " + tool.Title + " " + tool.Description
		}
		candidateTerms := mcpActivationTerms(haystack)
		for term := range queryTerms {
			if _, exists := candidateTerms[term]; !exists {
				continue
			}
			score++
			if _, nameMatch := nameTerms[term]; nameMatch {
				score++
			}
		}
		if score >= 2 {
			scored = append(scored, scoredServer{server: server, score: score})
		}
	}
	sort.Slice(scored, func(left, right int) bool {
		if scored[left].score == scored[right].score {
			return scored[left].server.Ref.Key() < scored[right].server.Ref.Key()
		}
		return scored[left].score > scored[right].score
	})
	if len(scored) > maxRunOnlyMCPActivations {
		scored = scored[:maxRunOnlyMCPActivations]
	}
	result := make([]SelectionServer, 0, len(scored))
	for _, item := range scored {
		result = append(result, SelectionServer{Ref: item.server.Ref, DisabledTools: []string{}})
	}
	return result, nil
}

func mcpNameMentioned(value string, name string) bool {
	value = strings.ToLower(value)
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if mcpContainsHan([]rune(name)) {
		return strings.Contains(value, name)
	}
	for offset := 0; offset <= len(value)-len(name); {
		index := strings.Index(value[offset:], name)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(name)
		leftBounded := index == 0 || !mcpIdentifierRune(previousRune(value[:index]))
		rightBounded := end == len(value) || !mcpIdentifierRune(nextRune(value[end:]))
		if leftBounded && rightBounded {
			return true
		}
		offset = index + len(name)
	}
	return false
}

func previousRune(value string) rune {
	r, _ := utf8.DecodeLastRuneInString(value)
	return r
}

func nextRune(value string) rune {
	r, _ := utf8.DecodeRuneInString(value)
	return r
}

func mcpIdentifierRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_'
}

func mcpActivationTerms(value string) map[string]struct{} {
	const maxTerms = 96
	stopwords := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "this": {},
		"that": {}, "use": {}, "using": {}, "agent": {}, "mcp": {}, "tool": {},
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
		if mcpContainsHan(runes) {
			for index := 0; index+1 < len(runes); index++ {
				add(string(runes[index : index+2]))
			}
		}
	}
	return terms
}

func mcpContainsHan(value []rune) bool {
	for _, item := range value {
		if unicode.Is(unicode.Han, item) {
			return true
		}
	}
	return false
}
