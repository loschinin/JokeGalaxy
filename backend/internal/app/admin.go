package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := s.requireUser(w, r)
	if !ok { return User{}, false }
	if user.Role != "admin" { writeJSON(w, http.StatusForbidden, map[string]string{"error": "Действие доступно только роли администратора."}); return User{}, false }
	return user, true
}

func (s *Server) handleAdminJokes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok { return }
	page, _ := strconv.Atoi(r.URL.Query().Get("page")); if page < 1 { page = 1 }
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")); if limit < 1 || limit > 100 { limit = 30 }
	if s.db != nil {
		items, err := s.queryJokes(r.Context(), jokeSelect+` ORDER BY j.created_at DESC LIMIT $1 OFFSET $2`, limit+1, (page-1)*limit)
		if err == nil { more := len(items)>limit; if more { items=items[:limit] }; writeJSON(w,http.StatusOK,map[string]any{"jokes":items,"has_more":more,"page":page}); return }
		writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"Не удалось загрузить очередь шуток."}); return
	}
	s.mu.RLock(); items := make([]Joke,0,len(s.jokes)); for _, joke := range s.jokes { items=append(items,joke) }; s.mu.RUnlock()
	sortJokes(items,"newest")
	start := (page-1)*limit; if start>=len(items) { writeJSON(w,http.StatusOK,map[string]any{"jokes":[]Joke{},"has_more":false,"page":page}); return }
	end:=start+limit+1; if end>len(items) { end=len(items) }
	batch:=items[start:end]; more:=len(batch)>limit; if more { batch=batch[:limit] }
	writeJSON(w,http.StatusOK,map[string]any{"jokes":batch,"has_more":more,"page":page})
}

func (s *Server) handleAdminUpdateJoke(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w,r); !ok { return }
	id:=pathID(r); if id==0 { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"Неверный номер шутки."}); return }
	var input struct { Text string `json:"text"`; Cluster string `json:"cluster"`; Triggers []string `json:"triggers"`; IsDuplicate *bool `json:"is_duplicate"` }
	if !decodeJSON(w,r,&input) { return }
	joke, exists := s.getJoke(r.Context(),id); if !exists { writeJSON(w,http.StatusNotFound,map[string]string{"error":"Шутка не найдена."}); return }
	if strings.TrimSpace(input.Text)!="" { joke.Text=strings.TrimSpace(input.Text) }
	if input.Cluster!="" { if cluster,ok:=s.clusters[input.Cluster]; ok { joke.Cluster=cluster } else { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"Неизвестный кластер."}); return } }
	if input.Triggers!=nil { joke.Triggers=sanitizeTriggers(input.Triggers) }
	if len(joke.Triggers)==0 { analysis:=s.analyze(r.Context(),joke.Text); joke.Triggers=analysis.Triggers }
	if input.IsDuplicate!=nil { joke.IsDuplicate=*input.IsDuplicate }
	vector,err:=s.ai.Embed(r.Context(),"Темы: "+strings.Join(joke.Triggers,", ")+". "+joke.Text,false)
	if err!=nil { vector=hashEmbedding("Темы: "+strings.Join(joke.Triggers,", ")+". "+joke.Text) }
	position:=s.placeCoordinates(vector)
	joke.Embedding=embeddingBytes(vector); joke.PosX,joke.PosY,joke.PosZ=position[0],position[1],position[2]
	if s.db!=nil {
		_,err=s.db.ExecContext(r.Context(),`UPDATE jokes SET text=$1,triggers=$2,cluster=$3,pos_x=$4,pos_y=$5,pos_z=$6,embedding=$7,is_duplicate=$8 WHERE id=$9`,joke.Text,joke.Triggers,joke.Cluster.Slug,joke.PosX,joke.PosY,joke.PosZ,joke.Embedding,joke.IsDuplicate,id)
		if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"Не удалось сохранить изменения."}); return }
	}
	joke.ID=strconv.FormatInt(id,10)
	s.cacheJoke(joke)
	writeJSON(w,http.StatusOK,joke)
}

func (s *Server) handleAdminDeleteJoke(w http.ResponseWriter,r *http.Request) {
	if _,ok:=s.requireAdmin(w,r); !ok { return }
	id:=pathID(r); if id==0 { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"Неверный номер шутки."}); return }
	if s.db!=nil {
		result,err:=s.db.ExecContext(r.Context(),`DELETE FROM jokes WHERE id=$1`,id)
		if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"Не удалось удалить шутку."}); return }
		count,_:=result.RowsAffected(); if count==0 { writeJSON(w,http.StatusNotFound,map[string]string{"error":"Шутка не найдена."}); return }
	}
	s.mu.Lock(); delete(s.jokes,id); delete(s.vectors,id); delete(s.coordinates,id)
	for relation:=range s.likes { if relation.JokeID==id { delete(s.likes,relation) } }
	for relation:=range s.favorites { if relation.JokeID==id { delete(s.favorites,relation) } }
	s.mu.Unlock()
	writeJSON(w,http.StatusOK,map[string]bool{"deleted":true})
}

func (s *Server) handleAdminPayments(w http.ResponseWriter,r *http.Request) {
	if _,ok:=s.requireAdmin(w,r); !ok { return }
	if s.db==nil { writeJSON(w,http.StatusOK,map[string]any{"payments":[]any{}}); return }
	rows,err:=s.db.QueryContext(r.Context(),`SELECT p.id,p.user_id,u.username,p.package_code,p.credits_amount,p.amount_rub,p.status,p.created_at,p.paid_at FROM payments p JOIN users u ON u.id=p.user_id ORDER BY p.created_at DESC LIMIT 100`)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"Не удалось загрузить платежи."}); return }
	defer rows.Close()
	type paymentRow struct { ID int64 `json:"id"`; UserID int64 `json:"user_id"`; Username string `json:"username"`; Package string `json:"package_code"`; Credits int `json:"credits_amount"`; Amount float64 `json:"amount_rub"`; Status string `json:"status"`; CreatedAt time.Time `json:"created_at"`; PaidAt *time.Time `json:"paid_at,omitempty"` }
	items:=make([]paymentRow,0,32)
	for rows.Next() { var item paymentRow; if rows.Scan(&item.ID,&item.UserID,&item.Username,&item.Package,&item.Credits,&item.Amount,&item.Status,&item.CreatedAt,&item.PaidAt)==nil { items=append(items,item) } }
	writeJSON(w,http.StatusOK,map[string]any{"payments":items})
}
