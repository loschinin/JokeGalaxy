package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxCSVBytes = 10 << 20
const importRate = 15

type csvJoke struct { sourceID, text string; triggers []string; cluster string }

func (s *Server) handleCSVImport(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	if user.Role != "admin" { writeJSON(w, http.StatusForbidden, map[string]string{"error": "Импорт доступен только роли администратора."}); return }
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+1<<20)
	if err := r.ParseMultipartForm(maxCSVBytes); err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Не удалось прочитать CSV-файл размером до 10 МБ."}); return }
	file, header, err := r.FormFile("file")
	if err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Выберите CSV-файл."}); return }
	defer file.Close()
	if header.Size > maxCSVBytes { writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Размер CSV не должен превышать 10 МБ."}); return }
	reader := csv.NewReader(io.LimitReader(file, maxCSVBytes+1))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	rows, err := reader.ReadAll()
	if err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CSV повреждён. Проверьте разделитель «;» и кодировку UTF-8."}); return }
	if len(rows) == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "В файле нет строк для импорта."}); return }
	start := 0
	if len(rows[0]) > 0 && strings.EqualFold(strings.TrimSpace(rows[0][0]), "id") { start = 1 }
	items := make([]csvJoke, 0, len(rows)-start)
	for index, fields := range rows[start:] {
		if len(fields) < 4 { continue }
		text := strings.TrimSpace(fields[1])
		if text == "" { continue }
		if len([]rune(text)) > 1400 { continue }
		items = append(items, csvJoke{sourceID: strings.TrimSpace(fields[0]), text: text, triggers: parseCSVTriggers(fields[2]), cluster: strings.TrimSpace(fields[3])})
		if len(items) > 50000 { writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "В одном задании допускается не более 50 000 строк."}); return }
		_ = index
	}
	if len(items) == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Не нашлось строк с колонками id;text;triggers;cluster."}); return }
	jobID, err := newIdempotenceKey()
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось создать задание импорта."}); return }
	status := &ImportStatus{ID: jobID, Filename: header.Filename, Status: "queued", TotalRows: len(items), CreatedAt: time.Now()}
	if s.db != nil {
		var dbID int64
		if err := s.db.QueryRowContext(r.Context(), `INSERT INTO import_jobs (user_id,filename,status,total_rows) VALUES ($1,$2,$3,$4) RETURNING id`, user.ID, header.Filename, status.Status, status.TotalRows).Scan(&dbID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось создать задание импорта."})
			return
		}
		jobID = strconv.FormatInt(dbID, 10)
		status.ID = jobID
	}
	s.mu.Lock(); s.imports[jobID] = status; s.mu.Unlock()
	go s.processCSVImport(context.Background(), jobID, user, items)
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "status": "queued", "total_rows": len(items)})
}

func parseCSVTriggers(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '|' || r == '#' })
	return sanitizeTriggers(parts)
}

func (s *Server) processCSVImport(ctx context.Context, jobID string, user User, items []csvJoke) {
	s.setImportStatus(jobID, "processing", "")
	ticker := time.NewTicker(time.Second / importRate)
	defer ticker.Stop()
	failureCount := 0
	importErrors := make([]string, 0, 5)
	for index, item := range items {
		<-ticker.C
		clusterSlug := resolveClusterSlug(item.cluster, s.clusterList())
		triggers := item.triggers
		if len(triggers) < 3 {
			analysis := s.analyze(ctx, item.text)
			triggers = appendUnique(triggers, analysis.Triggers...)
			if clusterSlug == "" { clusterSlug = analysis.Cluster }
		}
		if clusterSlug == "" { clusterSlug = fallbackCluster(item.text) }
		cluster, ok := s.clusters[clusterSlug]
		if !ok { cluster = s.clusters["puns"] }
		if len(triggers) == 0 { triggers = fallbackTriggers(item.text) }
		vector, err := s.ai.Embed(ctx, "Темы: "+strings.Join(triggers, ", ")+". "+item.text, false)
		if err != nil { vector = hashEmbedding("Темы: " + strings.Join(triggers, ", ") + ". " + item.text) }
		position := s.placeCoordinates(vector)
		duplicate := s.isNearDuplicate(vector)
		joke, saveErr := s.importJoke(ctx, user, item.text, triggers, cluster, vector, position, duplicate)
		if saveErr != nil {
			failureCount++
			if len(importErrors) < cap(importErrors) { importErrors = append(importErrors, fmt.Sprintf("Строка %d: %v", index+1, saveErr)) }
		} else { s.cacheJoke(joke) }
		s.mu.Lock()
		if status := s.imports[jobID]; status != nil { status.Processed = index + 1 }
		processed := index + 1
		s.mu.Unlock()
		if s.db != nil && processed%10 == 0 { _, _ = s.db.ExecContext(ctx, `UPDATE import_jobs SET processed_rows=$1 WHERE id=$2`, processed, parseID(jobID)) }
	}
	finalStatus, statusMessage := "completed", ""
	if failureCount > 0 {
		statusMessage = fmt.Sprintf("Не сохранено строк: %d. %s", failureCount, strings.Join(importErrors, "; "))
		if failureCount > len(importErrors) { statusMessage += fmt.Sprintf("; и ещё %d ошибок", failureCount-len(importErrors)) }
		if failureCount == len(items) { finalStatus = "failed" }
	}
	s.setImportStatus(jobID, finalStatus, statusMessage)
	if s.db != nil { _, _ = s.db.ExecContext(ctx, `UPDATE import_jobs SET status=$1, processed_rows=$2, error_text=$3, finished_at=NOW() WHERE id=$4`, finalStatus, len(items), statusMessage, parseID(jobID)) }
}

func resolveClusterSlug(raw string, clusters []Cluster) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	for _, cluster := range clusters {
		if value == strings.ToLower(cluster.Slug) || value == strings.ToLower(cluster.Name) { return cluster.Slug }
	}
	return ""
}

func (s *Server) importJoke(ctx context.Context, user User, text string, triggers []string, cluster Cluster, vector []float32, position [3]float64, duplicate bool) (Joke, error) {
	joke := Joke{Text: text, Triggers: triggers, Cluster: cluster, Author: Author{ID: strconv.FormatInt(user.ID, 10), Username: user.Username, AvatarURL: user.AvatarURL}, UserID: user.ID, CreatedAt: time.Now(), PosX: position[0], PosY: position[1], PosZ: position[2], Embedding: embeddingBytes(vector), IsDuplicate: duplicate}
	if s.db != nil {
		var id int64
		if err := s.db.QueryRowContext(ctx, `INSERT INTO jokes (user_id,text,triggers,cluster,pos_x,pos_y,pos_z,embedding,is_duplicate) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,created_at`, user.ID, text, triggers, cluster.Slug, position[0], position[1], position[2], joke.Embedding, duplicate).Scan(&id, &joke.CreatedAt); err != nil { return Joke{}, err }
		joke.ID = strconv.FormatInt(id, 10)
	} else { joke.ID = strconv.FormatInt(s.newJokeID(), 10) }
	return joke, nil
}

func (s *Server) setImportStatus(jobID, state, message string) {
	s.mu.Lock()
	if status := s.imports[jobID]; status != nil { status.Status = state; status.Error = message }
	s.mu.Unlock()
}

func (s *Server) handleImportStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	if user.Role != "admin" { writeJSON(w, http.StatusForbidden, map[string]string{"error": "Недостаточно прав."}); return }
	jobID := r.PathValue("id")
	s.mu.RLock(); status := s.imports[jobID]; var copyStatus ImportStatus; if status != nil { copyStatus = *status }; s.mu.RUnlock()
	if status != nil { writeJSON(w, http.StatusOK, copyStatus); return }
	if s.db != nil {
		var result ImportStatus
		id, _ := strconv.ParseInt(jobID, 10, 64)
		err := s.db.QueryRowContext(r.Context(), `SELECT id::text,filename,status,total_rows,processed_rows,error_text,created_at FROM import_jobs WHERE id=$1`, id).Scan(&result.ID,&result.Filename,&result.Status,&result.TotalRows,&result.Processed,&result.Error,&result.CreatedAt)
		if err == nil { writeJSON(w, http.StatusOK, result); return }
		if err != sql.ErrNoRows { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось получить статус импорта."}); return }
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "Задание импорта не найдено."})
}
