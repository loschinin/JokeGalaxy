package app

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const jokeSelect = `
SELECT j.id, j.text, j.triggers, j.cluster, c.id, c.name, c.color, c.description,
       j.user_id, u.username, u.avatar_url, u.bio, j.likes_count, j.favorites_count,
       j.created_at, j.pos_x, j.pos_y, j.pos_z, j.embedding, j.is_duplicate
FROM jokes j
JOIN clusters c ON c.slug = j.cluster
JOIN users u ON u.id = j.user_id
`

type rowScanner interface { Scan(dest ...any) error }

func scanJoke(row rowScanner) (Joke, error) {
	var joke Joke
	var id, authorID, clusterID int64
	var clusterSlug, username, avatarURL, bio string
	var triggers []string
	var created time.Time
	var embedding []byte
	err := row.Scan(&id, &joke.Text, &triggers, &clusterSlug, &clusterID, &joke.Cluster.Name, &joke.Cluster.Color, &joke.Cluster.Description,
		&authorID, &username, &avatarURL, &bio, &joke.LikesCount, &joke.FavoritesCount,
		&created, &joke.PosX, &joke.PosY, &joke.PosZ, &embedding, &joke.IsDuplicate)
	if err != nil { return Joke{}, err }
	joke.ID = strconv.FormatInt(id, 10)
	joke.UserID = authorID
	joke.Triggers = triggers
	joke.Cluster.ID = clusterID
	joke.Cluster.Slug = clusterSlug
	joke.Author = Author{ID: strconv.FormatInt(authorID, 10), Username: username, AvatarURL: avatarURL, Bio: bio}
	joke.CreatedAt = created
	joke.Embedding = embedding
	return joke, nil
}

func (s *Server) getJoke(ctx context.Context, id int64) (Joke, bool) {
	if s.db != nil {
		joke, err := scanJoke(s.db.QueryRowContext(ctx, jokeSelect+` WHERE j.id = $1`, id))
		if err == nil { return joke, true }
		if err != sql.ErrNoRows { return Joke{}, false }
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	joke, ok := s.jokes[id]
	return joke, ok
}

func (s *Server) queryJokes(ctx context.Context, query string, args ...any) ([]Joke, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	result := make([]Joke, 0, 24)
	for rows.Next() {
		joke, err := scanJoke(rows)
		if err != nil { return nil, err }
		result = append(result, joke)
	}
	return result, rows.Err()
}

func (s *Server) listJokes(ctx context.Context, page, limit int, cluster, trigger, author string, sort string, favoritesOnly bool, userID int64) ([]Joke, bool) {
	if page < 1 { page = 1 }
	if limit < 1 { limit = 12 }
	if limit > 50 { limit = 50 }
	if s.db != nil {
		where := []string{"j.is_duplicate = FALSE"}
		args := make([]any, 0, 7)
		if cluster != "" { args = append(args, cluster); where = append(where, fmt.Sprintf("j.cluster = $%d", len(args))) }
		if trigger != "" { args = append(args, "%"+trigger+"%"); where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM unnest(j.triggers) tag WHERE tag ILIKE $%d)", len(args))) }
		if author != "" { authorID, err := strconv.ParseInt(author, 10, 64); if err == nil { args = append(args, authorID); where = append(where, fmt.Sprintf("j.user_id = $%d", len(args))) } }
		if favoritesOnly {
			if userID <= 0 { return []Joke{}, false }
			args = append(args, userID)
			where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM joke_favorites jf WHERE jf.joke_id = j.id AND jf.user_id = $%d)", len(args)))
		}
		order := map[string]string{
			"newest": "j.created_at DESC", "oldest": "j.created_at ASC", "most-liked": "j.likes_count DESC",
			"least-liked": "j.likes_count ASC", "most-saved": "j.favorites_count DESC", "random": "random()",
		}[sort]
		if order == "" { order = "j.created_at DESC" }
		args = append(args, limit+1)
		limitParam := len(args)
		args = append(args, (page-1)*limit)
		offsetParam := len(args)
		query := fmt.Sprintf("%s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d", jokeSelect, strings.Join(where, " AND "), order, limitParam, offsetParam)
		items, err := s.queryJokes(ctx, query, args...)
		if err == nil {
			hasMore := len(items) > limit
			if hasMore { items = items[:limit] }
			return items, hasMore
		}
	}
	items := s.memoryList(cluster, trigger, author, sort, favoritesOnly, userID)
	start := (page - 1) * limit
	if start >= len(items) { return []Joke{}, false }
	end := start + limit
	hasMore := end < len(items)
	if end > len(items) { end = len(items) }
	return items[start:end], hasMore
}

func (s *Server) memoryList(cluster, trigger, author, sort string, favoritesOnly bool, userID int64) []Joke {
	s.mu.RLock()
	items := make([]Joke, 0, len(s.jokes))
	for id, joke := range s.jokes {
		if joke.IsDuplicate { continue }
		if cluster != "" && joke.Cluster.Slug != cluster { continue }
		if trigger != "" && !containsFold(joke.Triggers, trigger) { continue }
		if author != "" && joke.Author.ID != author { continue }
		if favoritesOnly && !s.favorites[Relation{UserID: userID, JokeID: id}] { continue }
		joke.Liked = s.likes[Relation{UserID: userID, JokeID: id}]
		joke.Favorited = s.favorites[Relation{UserID: userID, JokeID: id}]
		items = append(items, joke)
	}
	s.mu.RUnlock()
	sortJokes(items, sort)
	return items
}

func containsFold(values []string, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	for _, value := range values { if strings.Contains(strings.ToLower(value), query) { return true } }
	return false
}

func sortJokes(items []Joke, mode string) {
	less := func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) }
	switch mode {
	case "oldest": less = func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) }
	case "most-liked": less = func(i, j int) bool { return items[i].LikesCount > items[j].LikesCount }
	case "least-liked": less = func(i, j int) bool { return items[i].LikesCount < items[j].LikesCount }
	case "most-saved": less = func(i, j int) bool { return items[i].FavoritesCount > items[j].FavoritesCount }
	case "random": less = func(i, j int) bool { return items[i].ID < items[j].ID }
	}
	for i := 0; i < len(items); i++ { for j := i + 1; j < len(items); j++ { if less(j, i) { items[i], items[j] = items[j], items[i] } } }
}

func (s *Server) updateRelationDB(ctx context.Context, userID, jokeID int64, kind string) (bool, error) {
	if kind != "like" && kind != "favorite" { return false, fmt.Errorf("invalid relation kind") }
	if s.db != nil {
		table, counter := "joke_likes", "likes_count"
		if kind == "favorite" { table, counter = "joke_favorites", "favorites_count" }
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil { return false, err }
		insertResult, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (user_id, joke_id) VALUES ($1,$2) ON CONFLICT DO NOTHING", table), userID, jokeID)
		if err != nil { _ = tx.Rollback(); return false, err }
		inserted, _ := insertResult.RowsAffected()
		active := inserted > 0
		if inserted > 0 {
			_, err = tx.ExecContext(ctx, fmt.Sprintf("UPDATE jokes SET %s = %s + 1 WHERE id = $1", counter, counter), jokeID)
		} else {
			_, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE user_id = $1 AND joke_id = $2", table), userID, jokeID)
			if err == nil { _, err = tx.ExecContext(ctx, fmt.Sprintf("UPDATE jokes SET %s = GREATEST(%s - 1, 0) WHERE id = $1", counter, counter), jokeID) }
		}
		if err != nil { _ = tx.Rollback(); return false, err }
		if err := tx.Commit(); err != nil { return false, err }
		return active, nil
	}

	relation := Relation{UserID: userID, JokeID: jokeID}
	s.mu.Lock()
	defer s.mu.Unlock()
	joke, ok := s.jokes[jokeID]
	if !ok { return false, sql.ErrNoRows }
	collection := s.likes
	counter := &joke.LikesCount
	if kind == "favorite" { collection = s.favorites; counter = &joke.FavoritesCount }
	active := !collection[relation]
	if active { collection[relation] = true; *counter++ } else { delete(collection, relation); if *counter > 0 { *counter-- } }
	s.jokes[jokeID] = joke
	return active, nil
}
