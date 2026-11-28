package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"clusters": s.clusterList()})
}

func (s *Server) handleListJokes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit == 0 { limit = 12 }
	cluster, trigger := query.Get("cluster"), query.Get("tag")
	author := query.Get("author")
	favoritesOnly := query.Get("favorites") == "true" || query.Get("only_favorites") == "true"
	userID := s.currentUserID(r)
	items, hasMore := s.listJokes(r.Context(), page, limit, cluster, trigger, author, query.Get("sort"), favoritesOnly, userID)
	if userID > 0 {
		for index := range items {
			id := parseID(items[index].ID)
			if s.db != nil {
				_ = s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM joke_likes WHERE user_id=$1 AND joke_id=$2), EXISTS(SELECT 1 FROM joke_favorites WHERE user_id=$1 AND joke_id=$2)`, userID, id).Scan(&items[index].Liked, &items[index].Favorited)
			} else {
				s.mu.RLock(); items[index].Liked = s.likes[Relation{UserID: userID, JokeID: id}]; items[index].Favorited = s.favorites[Relation{UserID: userID, JokeID: id}]; s.mu.RUnlock()
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jokes": items, "has_more": hasMore, "page": maxInt(page, 1)})
}

func (s *Server) handleGetJoke(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный номер шутки."}); return }
	joke, ok := s.getJoke(r.Context(), id)
	if !ok { writeJSON(w, http.StatusNotFound, map[string]string{"error": "Шутка не найдена."}); return }
	if userID := s.currentUserID(r); userID > 0 {
		if s.db != nil { _ = s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM joke_likes WHERE user_id=$1 AND joke_id=$2), EXISTS(SELECT 1 FROM joke_favorites WHERE user_id=$1 AND joke_id=$2)`, userID, id).Scan(&joke.Liked, &joke.Favorited) } else {
			s.mu.RLock(); joke.Liked = s.likes[Relation{UserID: userID, JokeID: id}]; joke.Favorited = s.favorites[Relation{UserID: userID, JokeID: id}]; s.mu.RUnlock()
		}
	}
	writeJSON(w, http.StatusOK, joke)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 300 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Запрос слишком длинный."}); return }
	result := s.search(r.Context(), query, r.URL.Query().Get("mode"))
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePopularTriggers(w http.ResponseWriter, r *http.Request) {
	counts := make(map[string]int)
	s.mu.RLock()
	for _, joke := range s.jokes { for _, trigger := range joke.Triggers { counts[trigger]++ } }
	s.mu.RUnlock()
	type triggerCount struct { Tag string `json:"tag"`; Count int `json:"count"` }
	items := make([]triggerCount, 0, len(counts))
	for tag, count := range counts { items = append(items, triggerCount{tag, count}) }
	sort.Slice(items, func(i, j int) bool { if items[i].Count == items[j].Count { return items[i].Tag < items[j].Tag }; return items[i].Count > items[j].Count })
	if len(items) > 20 { items = items[:20] }
	writeJSON(w, http.StatusOK, map[string]any{"triggers": items})
}

func (s *Server) handleAnalyzeJoke(w http.ResponseWriter, r *http.Request) {
	var input struct { Text string `json:"text"` }
	if !decodeJSON(w, r, &input) { return }
	input.Text = strings.TrimSpace(input.Text)
	if len([]rune(input.Text)) < 8 || len([]rune(input.Text)) > 1400 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Добавьте шутку длиной от 8 до 1400 символов."}); return }
	analysis := s.analyze(r.Context(), input.Text)
	writeJSON(w, http.StatusOK, analysis)
}

func (s *Server) analyze(ctx context.Context, text string) struct { Triggers []string `json:"triggers"`; Cluster string `json:"cluster"` } {
	if result, err := s.ai.Analyze(ctx, text, clusterSlugs(s.clusterList())); err == nil {
		clusterExists := false
		for _, cluster := range s.clusterList() { if cluster.Slug == result.Cluster { clusterExists = true; break } }
		if !clusterExists { result.Cluster = fallbackCluster(text) }
		triggers := sanitizeTriggers(result.Triggers)
		if len(triggers) < 3 { triggers = appendUnique(triggers, fallbackTriggers(text)...); triggers = sanitizeTriggers(triggers) }
		return struct { Triggers []string `json:"triggers"`; Cluster string `json:"cluster"` }{Triggers: triggers, Cluster: result.Cluster}
	}
	return fallbackAnalysis(text)
}

func fallbackAnalysis(text string) struct { Triggers []string `json:"triggers"`; Cluster string `json:"cluster"` } {
	triggers := fallbackTriggers(text)
	if len(triggers) > 8 { triggers = triggers[:8] }
	return struct { Triggers []string `json:"triggers"`; Cluster string `json:"cluster"` }{Triggers: triggers, Cluster: fallbackCluster(text)}
}

func fallbackCluster(text string) string {
	lower := strings.ToLower(text)
	keywords := map[string][]string{
		"it": {"програм", "код", "сервер", "баг", "интернет", "нейросет", "сайт", "компьют", "прилож"},
		"relationships": {"муж", "жен", "свидан", "любов", "семь", "тёщ", "девуш", "парень", "секс"},
		"medicine": {"врач", "больниц", "здоров", "аптек", "психолог", "стоматолог", "диет"},
		"work-money": {"работ", "офис", "зарплат", "деньг", "кредит", "начальник", "коллег", "бизнес"},
		"politics": {"налог", "чинов", "государств", "дорог", "жкх", "выбор", "закон", "политик"},
		"study-army": {"школ", "учител", "экзамен", "студент", "арм", "егэ", "военком"},
		"heroes": {"штирлиц", "кино", "фильм", "сказк", "герой", "дракон", "книг", "животн"},
		"dark-philosophy": {"жизн", "смерт", "старост", "похорон", "смысл", "философ", "цинизм"},
	}
	best, bestScore := "puns", 0
	for cluster, terms := range keywords {
		score := 0
		for _, term := range terms { if strings.Contains(lower, term) { score++ } }
		if score > bestScore { best, bestScore = cluster, score }
	}
	return best
}

func fallbackTriggers(text string) []string {
	unique := make([]string, 0, 8)
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len([]rune(token)) >= 4 { unique = appendUnique(unique, token) }
		if len(unique) >= 8 { break }
	}
	return unique
}

func sanitizeTriggers(values []string) []string {
	out := make([]string, 0, 8)
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
		if value == "" || len([]rune(value)) > 36 { continue }
		out = appendUnique(out, value)
		if len(out) == 8 { break }
	}
	return out
}

func appendUnique(values []string, extras ...string) []string {
	seen := make(map[string]bool, len(values)+len(extras))
	for _, value := range values { seen[strings.ToLower(value)] = true }
	for _, value := range extras {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] { continue }
		seen[strings.ToLower(value)] = true
		values = append(values, value)
	}
	return values
}

func clusterSlugs(clusters []Cluster) []string { values := make([]string, 0, len(clusters)); for _, cluster := range clusters { values = append(values, cluster.Slug) }; return values }

func (s *Server) handleCreateJoke(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	var input JokeInput
	if !decodeJSON(w, r, &input) { return }
	input.Text = strings.TrimSpace(input.Text)
	if len([]rune(input.Text)) < 8 || len([]rune(input.Text)) > 1400 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Текст шутки должен содержать от 8 до 1400 символов."}); return }
	if len(input.Triggers) == 0 { analysis := s.analyze(r.Context(), input.Text); input.Triggers = analysis.Triggers; if input.Cluster == "" { input.Cluster = analysis.Cluster } }
	input.Triggers = sanitizeTriggers(input.Triggers)
	cluster, exists := s.clusters[input.Cluster]
	if !exists { cluster = s.clusters[fallbackCluster(input.Text)] }
	if cluster.Slug == "" { cluster = s.clusters["puns"] }
	document := embeddingText(input.Text, input.Triggers)
	vector, err := s.ai.Embed(r.Context(), document, false)
	embedding := semanticEmbeddingBytes(vector, err)
	if embedding == nil { vector = hashEmbedding(document) }
	position := [3]float64{}
	if embedding != nil { position = s.placeCoordinates(vector) }
	duplicate := s.isNearDuplicate(vector)
	joke := Joke{Text: input.Text, Triggers: input.Triggers, Cluster: cluster, Author: Author{ID: strconv.FormatInt(user.ID, 10), Username: user.Username, AvatarURL: user.AvatarURL, Bio: user.Bio}, CreatedAt: time.Now(), PosX: position[0], PosY: position[1], PosZ: position[2], LikesCount: 0, FavoritesCount: 0, IsDuplicate: duplicate, Embedding: embedding, UserID: user.ID}
	if s.db != nil {
		var id int64
		err := s.db.QueryRowContext(r.Context(), `INSERT INTO jokes (user_id,text,triggers,cluster,pos_x,pos_y,pos_z,embedding,is_duplicate) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,created_at`, user.ID, joke.Text, joke.Triggers, cluster.Slug, joke.PosX, joke.PosY, joke.PosZ, joke.Embedding, duplicate).Scan(&id, &joke.CreatedAt)
		if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось сохранить шутку."}); return }
		joke.ID = strconv.FormatInt(id, 10)
	} else {
		id := s.newJokeID()
		joke.ID = strconv.FormatInt(id, 10)
	}
	s.cacheJoke(joke)
	s.initializePCABasis()
	s.startMissingEmbeddingReindex()
	writeJSON(w, http.StatusCreated, joke)
}

func (s *Server) handleLike(w http.ResponseWriter, r *http.Request) { s.handleJokeRelation(w, r, "like") }
func (s *Server) handleFavorite(w http.ResponseWriter, r *http.Request) { s.handleJokeRelation(w, r, "favorite") }

func (s *Server) handleJokeRelation(w http.ResponseWriter, r *http.Request, kind string) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	id := pathID(r)
	if id == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный номер шутки."}); return }
	active, err := s.updateRelationDB(r.Context(), user.ID, id, kind)
	if err != nil { if err == sql.ErrNoRows { writeJSON(w, http.StatusNotFound, map[string]string{"error": "Шутка не найдена."}) } else { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось сохранить действие."}) }; return }
	joke, _ := s.getJoke(r.Context(), id)
	field := "liked"
	if kind == "favorite" { field = "favorited" }
	writeJSON(w, http.StatusOK, map[string]any{field: active, "likes_count": joke.LikesCount, "favorites_count": joke.FavoritesCount})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := s.userFromRequest(r)
	if !ok { writeJSON(w, http.StatusOK, map[string]any{"authenticated": false}); return }
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user": user})
}

func (s *Server) handleGetPublicUser(w http.ResponseWriter, r *http.Request) {
	rawID := strings.TrimSpace(r.PathValue("id"))
	user, ok := s.getPublicUser(r.Context(), rawID)
	if !ok { writeJSON(w, http.StatusNotFound, map[string]string{"error": "Профиль не найден."}); return }
	jokeCount, likesCount := 0, 0
	if s.db != nil {
		if err := s.db.QueryRowContext(r.Context(), `SELECT count(*), COALESCE(sum(likes_count),0) FROM jokes WHERE user_id=$1 AND is_duplicate=FALSE`, user.ID).Scan(&jokeCount, &likesCount); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось загрузить профиль."}); return
		}
	} else {
		s.mu.RLock()
		for _, joke := range s.jokes {
			if joke.UserID == user.ID && !joke.IsDuplicate { jokeCount++; likesCount += joke.LikesCount }
		}
		s.mu.RUnlock()
	}
	profile := struct {
		ID int64 `json:"id"`
		Username string `json:"username"`
		AvatarURL string `json:"avatar_url,omitempty"`
		Bio string `json:"bio,omitempty"`
		CreatedAt time.Time `json:"created_at"`
		JokesCount int `json:"jokes_count"`
		LikesCount int `json:"likes_count"`
	}{ID:user.ID, Username:user.Username, AvatarURL:user.AvatarURL, Bio:user.Bio, CreatedAt:user.CreatedAt, JokesCount:jokeCount, LikesCount:likesCount}
	writeJSON(w, http.StatusOK, map[string]any{"user": profile})
}

func (s *Server) handleOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	if err := s.onboardingComplete(r.Context(), user.ID); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось сохранить статус знакомства."}); return }
	writeJSON(w, http.StatusOK, map[string]bool{"onboarding_completed": true})
}

func (s *Server) handleSuggestJokes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok { return }
	var input struct { Story string `json:"story"` }
	if !decodeJSON(w, r, &input) { return }
	input.Story = strings.TrimSpace(input.Story)
	if len([]rune(input.Story)) < 12 || len([]rune(input.Story)) > 3500 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "История должна содержать от 12 до 3500 символов."}); return }
	themes, err := s.ai.ExtractThemes(r.Context(), input.Story)
	if err != nil { themes = fallbackThemes(input.Story) }
	if len(themes) == 0 { themes = fallbackThemes(input.Story) }
	collected := make([]Joke, 0, 12)
	seen := make(map[string]bool)
	for _, theme := range themes {
		result := s.search(r.Context(), theme, "hybrid")
		for _, joke := range append(result.KeywordResults, result.SemanticResults...) {
			if seen[joke.ID] { continue }
			seen[joke.ID] = true
			collected = append(collected, joke)
		}
	}
	if len(collected) == 0 { result := s.search(r.Context(), input.Story, "semantic"); collected = result.SemanticResults }
	if len(collected) > 8 { collected = collected[:8] }
	writeJSON(w, http.StatusOK, map[string]any{"themes": themes, "jokes": collected})
}

func fallbackThemes(story string) []string {
	words := fallbackTriggers(story)
	if len(words) > 3 { words = words[:3] }
	return words
}

func (s *Server) handleGenerateMonologue(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	var input struct { Story string `json:"story"`; Style string `json:"style"`; JokeIDs []string `json:"joke_ids"` }
	if !decodeJSON(w, r, &input) { return }
	input.Story = strings.TrimSpace(input.Story)
	if len([]rune(input.Story)) < 12 || len([]rune(input.Story)) > 3500 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "История должна содержать от 12 до 3500 символов."}); return }
	if input.Style == "" { input.Style = "custom" }
	if input.Style != "custom" && input.Style != "carlin" && input.Style != "zhvanetsky" && input.Style != "carter" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неизвестный стиль подачи."}); return }
	if strings.TrimSpace(s.cfg.DeepSeekAPIKey) == "" { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Генерация монолога недоступна: настройте DeepSeek API."}); return }
	if !s.deductCredit(r.Context(), user.ID) { writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "На балансе нет генераций."}); return }

	selected := make([]Joke, 0, len(input.JokeIDs))
	for _, rawID := range input.JokeIDs {
		id := parseID(rawID)
		if id == 0 { continue }
		if joke, ok := s.getJoke(r.Context(), id); ok { selected = append(selected, joke) }
	}
	texts := make([]string, 0, len(selected))
	for _, joke := range selected { texts = append(texts, joke.Text) }
	flusher, canFlush := w.(http.Flusher)
	started := false
	generated := strings.Builder{}
	writeChunk := func(chunk string) error {
		if !started {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-transform")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		encoded, _ := json.Marshal(map[string]string{"text": chunk})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", encoded); err != nil { return err }
		if canFlush { flusher.Flush() }
		generated.WriteString(chunk)
		return nil
	}

	generationErr := s.ai.StreamMonologue(r.Context(), input.Story, input.Style, texts, writeChunk)
	if !started {
		s.refundCredit(r.Context(), user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Генерация не началась. Кредит возвращён на баланс."})
		return
	}
	if generationErr != nil {
		if canFlush { _, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"error":"Поток был прерван."}`); flusher.Flush() }
	} else {
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if canFlush { flusher.Flush() }
	}
	s.saveStandupScript(r.Context(), user.ID, input.Story, input.Style, input.JokeIDs, generated.String())
}

func (s *Server) deductCredit(ctx context.Context, userID int64) bool {
	if s.db != nil {
		var remaining int
		err := s.db.QueryRowContext(ctx, `UPDATE users SET standup_credits = standup_credits - 1 WHERE id = $1 AND standup_credits > 0 RETURNING standup_credits`, userID).Scan(&remaining)
		return err == nil
	}
	s.mu.Lock(); defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok || user.StandupCredits <= 0 { return false }
	user.StandupCredits--
	s.users[userID] = user
	return true
}

func (s *Server) refundCredit(ctx context.Context, userID int64) {
	if s.db != nil { _, _ = s.db.ExecContext(ctx, `UPDATE users SET standup_credits = standup_credits + 1 WHERE id = $1`, userID); return }
	s.mu.Lock(); defer s.mu.Unlock()
	if user, ok := s.users[userID]; ok { user.StandupCredits++; s.users[userID] = user }
}

func (s *Server) saveStandupScript(ctx context.Context, userID int64, story, style string, jokeIDs []string, generated string) {
	if s.db == nil { return }
	ids := make([]int64, 0, len(jokeIDs))
	for _, value := range jokeIDs { if id := parseID(value); id > 0 { ids = append(ids, id) } }
	_, _ = s.db.ExecContext(ctx, `INSERT INTO standup_scripts (user_id,original_story,style,used_joke_ids,generated_text) VALUES ($1,$2,$3,$4,$5)`, userID, story, style, ids, generated)
}

func maxInt(a, b int) int { if a > b { return a }; return b }
