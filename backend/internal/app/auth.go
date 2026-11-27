package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type tokenClaims struct {
	Subject string `json:"sub"`
	Role string `json:"role"`
	Expires int64 `json:"exp"`
	Issued int64 `json:"iat"`
}

func (s *Server) signToken(user User) string {
	now := time.Now()
	claims := tokenClaims{Subject: strconv.FormatInt(user.ID, 10), Role: user.Role, Issued: now.Unix(), Expires: now.Add(7 * 24 * time.Hour).Unix()}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) verifyToken(value string) (tokenClaims, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 { return tokenClaims{}, false }
	unsigned := parts[0] + "." + parts[1]
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil { return tokenClaims{}, false }
	mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	_, _ = mac.Write([]byte(unsigned))
	if !hmac.Equal(provided, mac.Sum(nil)) { return tokenClaims{}, false }
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil { return tokenClaims{}, false }
	var claims tokenClaims
	if json.Unmarshal(payload, &claims) != nil || claims.Expires < time.Now().Unix() { return tokenClaims{}, false }
	return claims, true
}

func (s *Server) userFromRequest(r *http.Request) (User, bool) {
	token := ""
	if cookie, err := r.Cookie("jg_session"); err == nil { token = cookie.Value }
	if token == "" {
		if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") { token = strings.TrimPrefix(header, "Bearer ") }
	}
	claims, ok := s.verifyToken(token)
	if !ok { return User{}, false }
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil { return User{}, false }
	if s.db != nil {
		var user User
		err := s.db.QueryRowContext(r.Context(), `SELECT id, COALESCE(yandex_id,''), email, username, avatar_url, bio, role, standup_credits, onboarding_completed, created_at FROM users WHERE id = $1`, id).Scan(
			&user.ID, &user.YandexID, &user.Email, &user.Username, &user.AvatarURL, &user.Bio, &user.Role, &user.StandupCredits, &user.OnboardingCompleted, &user.CreatedAt)
		if err == nil { return user, true }
		return User{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, exists := s.users[id]
	return user, exists
}

func (s *Server) currentUserID(r *http.Request) int64 {
	user, ok := s.userFromRequest(r)
	if !ok { return 0 }
	return user.ID
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := s.userFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Необходимо войти через Яндекс ID."})
		return User{}, false
	}
	return user, true
}

func (s *Server) secureCookie(r *http.Request) bool {
	if r.TLS != nil { return true }
	return strings.HasPrefix(strings.ToLower(s.cfg.SiteURL), "https://")
}

func randomState() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil { return "", err }
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func (s *Server) yandexLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.YandexClientID == "" || s.cfg.YandexClientSecret == "" {
		http.Error(w, "Яндекс ID не настроен на сервере.", http.StatusServiceUnavailable)
		return
	}
	state, err := randomState()
	if err != nil { http.Error(w, "Не удалось начать вход.", http.StatusInternalServerError); return }
	cookiePath := "/"
	http.SetCookie(w, &http.Cookie{Name: "jg_oauth_state", Value: state, Path: cookiePath, HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	redirectURI := s.cfg.SiteURL + "/auth/yandex/callback"
	query := url.Values{}
	query.Set("response_type", "code")
	query.Set("client_id", s.cfg.YandexClientID)
	query.Set("scope", "login:info login:email login:avatar")
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	http.Redirect(w, r, "https://oauth.yandex.ru/authorize?"+query.Encode(), http.StatusFound)
}

func (s *Server) yandexCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("jg_oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "Проверка входа не пройдена. Попробуйте ещё раз.", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "jg_oauth_state", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode})
	if code := r.URL.Query().Get("error"); code != "" { http.Error(w, "Вход через Яндекс отменён.", http.StatusUnauthorized); return }
	code := r.URL.Query().Get("code")
	if code == "" { http.Error(w, "Яндекс не вернул код авторизации.", http.StatusBadRequest); return }
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", s.cfg.YandexClientID)
	form.Set("client_secret", s.cfg.YandexClientSecret)
	tokenRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth.yandex.ru/token", strings.NewReader(form.Encode()))
	tokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenResponse, err := http.DefaultClient.Do(tokenRequest)
	if err != nil { http.Error(w, "Не удалось связаться с Яндекс ID.", http.StatusBadGateway); return }
	defer tokenResponse.Body.Close()
	var tokenData struct { AccessToken string `json:"access_token"`; TokenType string `json:"token_type"` }
	if tokenResponse.StatusCode < 200 || tokenResponse.StatusCode >= 300 || json.NewDecoder(tokenResponse.Body).Decode(&tokenData) != nil || tokenData.AccessToken == "" {
		http.Error(w, "Яндекс не подтвердил авторизацию.", http.StatusUnauthorized)
		return
	}
	profileRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://login.yandex.ru/info?format=json", nil)
	profileRequest.Header.Set("Authorization", "OAuth "+tokenData.AccessToken)
	profileResponse, err := http.DefaultClient.Do(profileRequest)
	if err != nil { http.Error(w, "Не удалось загрузить профиль Яндекса.", http.StatusBadGateway); return }
	defer profileResponse.Body.Close()
	var profile struct {
		ID string `json:"id"`
		Login string `json:"login"`
		DefaultEmail string `json:"default_email"`
		DisplayName string `json:"display_name"`
		RealName string `json:"real_name"`
		AvatarID string `json:"default_avatar_id"`
	}
	if profileResponse.StatusCode < 200 || profileResponse.StatusCode >= 300 || json.NewDecoder(profileResponse.Body).Decode(&profile) != nil || profile.ID == "" {
		http.Error(w, "Не удалось прочитать профиль Яндекса.", http.StatusBadGateway)
		return
	}
	username := profile.DisplayName
	if username == "" { username = profile.RealName }
	if username == "" { username = profile.Login }
	if username == "" { username = "Пользователь" }
	avatar := ""
	if profile.AvatarID != "" { avatar = "https://avatars.yandex.net/get-yapic/" + url.PathEscape(profile.AvatarID) + "/islands-200" }
	user, err := s.upsertYandexUser(ctx, profile.ID, profile.DefaultEmail, username, avatar)
	if err != nil { http.Error(w, "Не удалось сохранить профиль. Попробуйте позже.", http.StatusInternalServerError); return }
	jwt := s.signToken(user)
	http.SetCookie(w, &http.Cookie{Name: "jg_session", Value: jwt, Path: "/", HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode, MaxAge: int((7 * 24 * time.Hour).Seconds())})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) upsertYandexUser(ctx context.Context, yandexID, email, username, avatar string) (User, error) {
	if s.db != nil {
		var user User
		err := s.db.QueryRowContext(ctx, `
INSERT INTO users (yandex_id, email, username, avatar_url)
VALUES ($1, $2, $3, $4)
ON CONFLICT (yandex_id) DO UPDATE SET email = EXCLUDED.email, username = EXCLUDED.username, avatar_url = EXCLUDED.avatar_url
RETURNING id, yandex_id, email, username, avatar_url, bio, role, standup_credits, onboarding_completed, created_at`, yandexID, email, username, avatar).Scan(
			&user.ID, &user.YandexID, &user.Email, &user.Username, &user.AvatarURL, &user.Bio, &user.Role, &user.StandupCredits, &user.OnboardingCompleted, &user.CreatedAt)
		return user, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.yandexUsers[yandexID]; ok {
		user := s.users[id]
		user.Email, user.Username, user.AvatarURL = email, username, avatar
		s.users[id] = user
		return user, nil
	}
	id := s.nextUserID
	s.nextUserID++
	user := User{ID: id, YandexID: yandexID, Email: email, Username: username, AvatarURL: avatar, Role: "user", StandupCredits: 50, CreatedAt: time.Now()}
	s.users[id] = user
	s.yandexUsers[yandexID] = id
	return user, nil
}

func (s *Server) refreshUserCredits(ctx context.Context, userID int64) (int, error) {
	if s.db != nil { var credits int; err := s.db.QueryRowContext(ctx, `SELECT standup_credits FROM users WHERE id = $1`, userID).Scan(&credits); return credits, err }
	s.mu.RLock(); defer s.mu.RUnlock()
	user, ok := s.users[userID]
	if !ok { return 0, errors.New("user not found") }
	return user.StandupCredits, nil
}

func (s *Server) onboardingComplete(ctx context.Context, userID int64) error {
	if s.db != nil { _, err := s.db.ExecContext(ctx, `UPDATE users SET onboarding_completed = TRUE WHERE id = $1`, userID); return err }
	s.mu.Lock(); defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok { return errors.New("user not found") }
	user.OnboardingCompleted = true
	s.users[userID] = user
	return nil
}

func bearerFromRequest(r *http.Request) string {
	if value := r.Header.Get("Authorization"); strings.HasPrefix(value, "Bearer ") { return strings.TrimSpace(strings.TrimPrefix(value, "Bearer ")) }
	cookie, _ := r.Cookie("jg_session")
	if cookie == nil { return "" }
	return cookie.Value
}

func parseID(value string) int64 { id, _ := strconv.ParseInt(value, 10, 64); return id }
