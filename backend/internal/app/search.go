package app

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

func (s *Server) search(ctx context.Context, query, mode string) SearchResponse {
	query = strings.TrimSpace(query)
	mode = normalizeMode(mode)
	response := SearchResponse{KeywordResults: []Joke{}, SemanticResults: []Joke{}, Results: []Joke{}, Query: query, Mode: mode}
	if query == "" { return response }
	if mode == "hybrid" || mode == "text" { response.KeywordResults = s.keywordSearch(ctx, query, 24) }
	if mode == "hybrid" || mode == "semantic" {
		excluded := make(map[string]bool, len(response.KeywordResults))
		for _, joke := range response.KeywordResults { excluded[joke.ID] = true }
		response.SemanticResults = s.semanticSearch(ctx, query, excluded, 24)
	}
	response.Results = append(response.Results, response.KeywordResults...)
	response.Results = append(response.Results, response.SemanticResults...)
	return response
}

func normalizeMode(mode string) string {
	switch mode { case "text", "semantic": return mode; default: return "hybrid" }
}

func (s *Server) keywordSearch(ctx context.Context, query string, limit int) []Joke {
	if s.db != nil {
		const sqlQuery = `
` + jokeSelect + `
WHERE j.is_duplicate = FALSE AND (
    j.search_tsv @@ websearch_to_tsquery('russian', $1)
    OR j.text % $1
    OR j.text ILIKE '%' || $1 || '%'
    OR EXISTS (SELECT 1 FROM unnest(j.triggers) AS tag WHERE tag ILIKE '%' || $1 || '%')
)
ORDER BY (ts_rank_cd(j.search_tsv, websearch_to_tsquery('russian', $1)) * 4 + similarity(j.text, $1)) DESC,
         j.likes_count DESC
LIMIT $2`
		items, err := s.queryJokes(ctx, sqlQuery, query, limit)
		if err == nil {
			for index := range items {
				items[index].Relevance = lexicalRelevance(query, items[index])
			}
			return items
		}
	}

	s.mu.RLock()
	items := make([]Joke, 0, len(s.jokes))
	for _, joke := range s.jokes {
		if joke.IsDuplicate { continue }
		score := lexicalRelevance(query, joke)
		if score <= 0 { continue }
		joke.Relevance = score
		items = append(items, joke)
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].Relevance == items[j].Relevance { return items[i].LikesCount > items[j].LikesCount }
		return items[i].Relevance > items[j].Relevance
	})
	if len(items) > limit { items = items[:limit] }
	return items
}

func lexicalRelevance(query string, joke Joke) float64 {
	queryLower := strings.ToLower(query)
	textLower := strings.ToLower(joke.Text)
	if strings.Contains(textLower, queryLower) { return .99 }
	score := 0.0
	for _, token := range words(query) {
		if len([]rune(token)) < 2 { continue }
		if strings.Contains(textLower, token) { score += .22 }
		for _, trigger := range joke.Triggers {
			if strings.Contains(strings.ToLower(trigger), token) || strings.Contains(token, strings.ToLower(trigger)) { score += .34; break }
		}
		if strings.Contains(strings.ToLower(joke.Cluster.Name), token) { score += .12 }
	}
	if score == 0 { score = trigramSimilarity(queryLower, textLower) * .35 }
	return math.Min(.98, score)
}

func words(value string) []string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	return fields
}

func trigramSimilarity(left, right string) float64 {
	left = "  " + strings.ToLower(left) + "  "
	right = "  " + strings.ToLower(right) + "  "
	if len(left) < 3 || len(right) < 3 { return 0 }
	grams := make(map[string]int)
	for index := 0; index+3 <= len(left); index++ { grams[left[index:index+3]]++ }
	intersection, total := 0, 0
	for index := 0; index+3 <= len(right); index++ {
		gram := right[index:index+3]
		if grams[gram] > 0 { intersection++; grams[gram]-- }
		total++
	}
	denominator := len(left) - 2 + total
	if denominator == 0 { return 0 }
	return 2 * float64(intersection) / float64(denominator)
}

func (s *Server) semanticSearch(ctx context.Context, query string, excluded map[string]bool, limit int) []Joke {
	vectorContext, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	vector, err := s.ai.Embed(vectorContext, query, true)
	if err != nil { vector = hashEmbedding(query) }

	type result struct { joke Joke; score float64 }
	s.mu.RLock()
	matches := make([]result, 0, len(s.jokes))
	for id, candidateVector := range s.vectors {
		joke, ok := s.jokes[id]
		if !ok || joke.IsDuplicate || excluded[joke.ID] { continue }
		score := dot(vector, candidateVector)
		matches = append(matches, result{joke: joke, score: score})
	}
	s.mu.RUnlock()
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > limit { matches = matches[:limit] }
	out := make([]Joke, 0, len(matches))
	for _, match := range matches {
		match.joke.Relevance = math.Max(0, math.Min(1, (match.score+1)/2))
		out = append(out, match.joke)
	}
	return out
}

func (s *Server) nearestJokes(vector []float32, limit int, excludeID string) []Joke {
	type result struct { joke Joke; score float64 }
	s.mu.RLock()
	matches := make([]result, 0, len(s.jokes))
	for id, cached := range s.vectors {
		joke := s.jokes[id]
		if joke.ID == excludeID || joke.IsDuplicate { continue }
		matches = append(matches, result{joke: joke, score: dot(vector, cached)})
	}
	s.mu.RUnlock()
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > limit { matches = matches[:limit] }
	out := make([]Joke, 0, len(matches))
	for _, match := range matches { match.joke.Relevance = math.Max(0, math.Min(1, (match.score+1)/2)); out = append(out, match.joke) }
	return out
}

func (s *Server) isNearDuplicate(vector []float32) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cached := range s.vectors { if dot(vector, cached) >= .91 { return true } }
	return false
}
