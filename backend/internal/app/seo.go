package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	styleTagPattern = regexp.MustCompile(`<link[^>]*rel="stylesheet"[^>]*>`)
	scriptTagPattern = regexp.MustCompile(`<script[^>]*type="module"[^>]*></script>`)
)

type seoPageData struct {
	Title string
	Description string
	Canonical string
	Content template.HTML
	Styles template.HTML
	Scripts template.HTML
	JSONLD template.JS
}

var seoPageTemplate = template.Must(template.New("seo").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="theme-color" content="#090b16">
<title>{{.Title}}</title><meta name="description" content="{{.Description}}"><link rel="canonical" href="{{.Canonical}}">
<meta property="og:type" content="article"><meta property="og:title" content="{{.Title}}"><meta property="og:description" content="{{.Description}}"><meta property="og:url" content="{{.Canonical}}"><meta property="og:site_name" content="Галактика юмора">
<link rel="manifest" href="/manifest.json">{{.Styles}}<script type="application/ld+json">{{.JSONLD}}</script></head>
<body><div id="seo-prerender">{{.Content}}</div><div id="root"></div>{{.Scripts}}</body></html>`))

var jokeSSRContent = template.Must(template.New("joke-content").Funcs(template.FuncMap{"urlquery": url.QueryEscape}).Parse(`<main style="max-width:920px;margin:0 auto;padding:32px 22px;color:#eee;font:16px/1.7 system-ui,sans-serif"><header style="display:flex;align-items:center;gap:10px;margin-bottom:28px"><a href="/" style="color:#f3cf82;text-decoration:none">✦ Галактика юмора</a><span style="color:#788">/</span><a href="/cluster/{{.Joke.Cluster.Slug}}" style="color:#aaa;text-decoration:none">{{.Joke.Cluster.Name}}</a></header><article><h1 style="font-size:clamp(28px,5vw,48px);line-height:1.35">{{.Joke.Text}}</h1><p>Автор: <a href="/user/{{.Joke.Author.ID}}" style="color:#c0aaff">{{.Joke.Author.Username}}</a> · {{.Joke.LikesCount}} отметок «нравится»</p><p>Кластер: <a href="/cluster/{{.Joke.Cluster.Slug}}" style="color:{{.Joke.Cluster.Color}}">{{.Joke.Cluster.Name}}</a></p><h2>Смысловые триггеры</h2><p>{{range $i, $tag := .Joke.Triggers}}{{if $i}} · {{end}}<a href="/tag/{{urlquery $tag}}" style="color:#b9a8ff">#{{$tag}}</a>{{end}}</p></article><section><h2>Похожие шутки</h2><ul>{{range .Similar}}<li><a href="/joke/{{.ID}}">{{.Text}}</a> <small>— {{.Cluster.Name}}</small></li>{{else}}<li>Похожие шутки скоро появятся.</li>{{end}}</ul></section><nav><a href="/cluster/{{.Joke.Cluster.Slug}}">Ещё из этого кластера</a> · <a href="/">Открыть интерактивную галактику</a></nav></main>`))

var collectionSSRContent = template.Must(template.New("collection-content").Funcs(template.FuncMap{"urlquery": url.QueryEscape}).Parse(`<main style="max-width:1000px;margin:0 auto;padding:32px 22px;color:#eee;font:16px/1.7 system-ui,sans-serif"><header><a href="/" style="color:#f3cf82;text-decoration:none">✦ Галактика юмора</a></header><h1>{{.Title}}</h1><p>{{.Description}}</p><p>{{.Intro}}</p><section><ul>{{range .Jokes}}<li><a href="/joke/{{.ID}}">{{.Text}}</a><small> · <a href="/user/{{.Author.ID}}">{{.Author.Username}}</a> · <a href="/cluster/{{.Cluster.Slug}}">{{.Cluster.Name}}</a>{{range .Triggers}} · <a href="/tag/{{urlquery .}}">#{{.}}</a>{{end}}</small></li>{{else}}<li>В этой подборке пока нет шуток.</li>{{end}}</ul></section><nav aria-label="Страницы">{{if .Previous}}<a href="?page={{.Previous}}">← Назад</a>{{end}} <span>Страница {{.Page}}</span> {{if .Next}}<a href="?page={{.Next}}">Дальше →</a>{{end}}</nav></main>`))

var profileSSRContent = template.Must(template.New("profile-content").Parse(`<main style="max-width:1000px;margin:0 auto;padding:32px 22px;color:#eee;font:16px/1.7 system-ui,sans-serif"><header><a href="/" style="color:#f3cf82;text-decoration:none">✦ Галактика юмора</a></header><h1>{{.User.Username}}</h1><p>{{.User.Bio}}</p><p>{{.Count}} шуток · {{.Likes}} лайков · Автор с {{.Joined}}</p><h2>Публикации автора</h2><ul>{{range .Jokes}}<li><a href="/joke/{{.ID}}">{{.Text}}</a> · <a href="/cluster/{{.Cluster.Slug}}">{{.Cluster.Name}}</a></li>{{else}}<li>Публикаций пока нет.</li>{{end}}</ul><nav><a href="/">Исследовать галактику</a></nav></main>`))

func (s *Server) pageAssets() (template.HTML, template.HTML) {
	index, err := os.ReadFile(filepath.Join(s.frontendDist(), "index.html"))
	if err != nil { return "", "" }
	content := string(index)
	styles := styleTagPattern.FindAllString(content, -1)
	scripts := scriptTagPattern.FindAllString(content, -1)
	return template.HTML(strings.Join(styles, "")), template.HTML(strings.Join(scripts, ""))
}

func (s *Server) renderSEO(w http.ResponseWriter, title, description, path string, content []byte, jsonld any) {
	var body template.HTML
	if len(content) == 0 { body = template.HTML(`<main style="max-width:900px;margin:0 auto;padding:40px;color:#eee;font:16px system-ui"><a href="/">✦ Галактика юмора</a><h1>Материал временно недоступен</h1><p>Попробуйте обновить страницу чуть позже.</p></main>`) } else { body = template.HTML(content) }
	styles, scripts := s.pageAssets()
	var structured template.JS
	if jsonld != nil {
		if bytes, err := json.Marshal(jsonld); err == nil { structured = template.JS(bytes) }
	}
	data := seoPageData{Title: title, Description: description, Canonical: s.cfg.SiteURL + path, Content: body, Styles: styles, Scripts: scripts, JSONLD: structured}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "index, follow")
	_ = seoPageTemplate.Execute(w, data)
}

func (s *Server) handleSSRJoke(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	joke, ok := s.getJoke(r.Context(), id)
	if !ok { http.NotFound(w, r); return }
	vector := vectorFromBytes(joke.Embedding)
	if vector == nil { s.mu.RLock(); vector = append([]float32(nil), s.vectors[id]...); s.mu.RUnlock() }
	similar := s.nearestJokes(vector, 4, joke.ID)
	var body strings.Builder
	_ = jokeSSRContent.Execute(&body, struct { Joke Joke; Similar []Joke }{joke, similar})
	title := joke.Text
	if index := strings.IndexByte(title, '\n'); index >= 0 { title = title[:index] }
	title = strings.TrimSpace(title)
	if len([]rune(title)) > 78 { title = string([]rune(title)[:75]) + "…" }
	title += " — шутка из Галактики юмора"
	description := strings.Join(strings.Fields(joke.Text), " ")
	if len([]rune(description)) > 155 { description = string([]rune(description)[:152]) + "…" }
	ld := map[string]any{"@context": "https://schema.org", "@type": "CreativeWork", "name": title, "text": joke.Text, "author": map[string]string{"@type": "Person", "name": joke.Author.Username, "url": s.cfg.SiteURL + "/user/" + url.PathEscape(joke.Author.ID)}, "keywords": joke.Triggers, "genre": joke.Cluster.Name, "url": s.cfg.SiteURL + "/joke/" + joke.ID}
	s.renderSEO(w, title, description, "/joke/"+url.PathEscape(joke.ID), []byte(body.String()), ld)
}

func (s *Server) handleSSRCluster(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var cluster Cluster
	for _, candidate := range s.clusterList() { if candidate.Slug == slug { cluster = candidate; break } }
	if cluster.Slug == "" { http.NotFound(w, r); return }
	page := queryPage(r)
	items, hasMore := s.listJokes(r.Context(), page, 20, slug, "", "", "newest", false, 0)
	next, previous := 0, 0
	if hasMore { next = page + 1 }
	if page > 1 { previous = page - 1 }
	data := map[string]any{"Title": cluster.Name, "Description": "Русские шутки, каламбуры и наблюдения на тему «" + cluster.Name + "».", "Intro": cluster.Description, "Jokes": items, "Page": page, "Next": next, "Previous": previous}
	var body strings.Builder
	_ = collectionSSRContent.Execute(&body, data)
	path := "/cluster/" + url.PathEscape(slug)
	s.renderSEO(w, cluster.Name+" — русские шутки и каламбуры", data["Description"].(string), path+pageQuery(page), []byte(body.String()), map[string]any{"@context": "https://schema.org", "@type": "CollectionPage", "name": cluster.Name, "url": s.cfg.SiteURL + path})
}

func (s *Server) handleSSRTag(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	if strings.TrimSpace(tag) == "" { http.NotFound(w, r); return }
	page := queryPage(r)
	items, hasMore := s.listJokes(r.Context(), page, 20, "", tag, "", "newest", false, 0)
	next, previous := 0, 0
	if hasMore { next = page + 1 }; if page > 1 { previous = page - 1 }
	description := "Русские шутки по смысловому тегу «" + tag + "». Найдите каламбуры, истории и наблюдения на эту тему."
	title := "Шутки по тегу «" + tag + "»"
	data := map[string]any{"Title": title, "Description": description, "Intro": "Тег объединяет шутки с похожими смысловыми триггерами.", "Jokes": items, "Page": page, "Next": next, "Previous": previous}
	var body strings.Builder; _ = collectionSSRContent.Execute(&body, data)
	path := "/tag/" + url.PathEscape(tag)
	s.renderSEO(w, title+" — Галактика юмора", description, path+pageQuery(page), []byte(body.String()), map[string]any{"@context": "https://schema.org", "@type": "CollectionPage", "name": title, "url": s.cfg.SiteURL + path})
}

func (s *Server) handleSSRUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, ok := s.getPublicUser(r.Context(), id)
	if !ok { http.NotFound(w, r); return }
	items, _ := s.listJokes(r.Context(), 1, 100, "", "", id, "newest", false, 0)
	likes := 0
	for _, joke := range items { likes += joke.LikesCount }
	joined := russianMonth(user.CreatedAt)
	data := map[string]any{"User": user, "Jokes": items, "Count": len(items), "Likes": likes, "Joined": joined}
	var body strings.Builder; _ = profileSSRContent.Execute(&body, data)
	title := user.Username + " — автор Галактики юмора"
	description := user.Bio
	if description == "" { description = user.Username + " публикует русские шутки, каламбуры и истории в Галактике юмора." }
	path := "/user/" + url.PathEscape(id)
	s.renderSEO(w, title, description, path, []byte(body.String()), map[string]any{"@context": "https://schema.org", "@type": "ProfilePage", "mainEntity": map[string]any{"@type": "Person", "name": user.Username, "description": user.Bio, "url": s.cfg.SiteURL + path}})
}

func (s *Server) getPublicUser(ctx context.Context, rawID string) (User, bool) {
	if s.db != nil {
		var user User
		err := s.db.QueryRowContext(ctx, `SELECT id,COALESCE(yandex_id,''),email,username,avatar_url,bio,role,standup_credits,onboarding_completed,created_at FROM users WHERE id::text=$1`, rawID).Scan(&user.ID,&user.YandexID,&user.Email,&user.Username,&user.AvatarURL,&user.Bio,&user.Role,&user.StandupCredits,&user.OnboardingCompleted,&user.CreatedAt)
		if err == nil { return user, true }
		return User{}, false
	}
	id := parseID(rawID)
	s.mu.RLock(); defer s.mu.RUnlock()
	user, ok := s.users[id]
	return user, ok
}

func russianMonth(value time.Time) string {
	months := [...]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"}
	return months[int(value.Month())-1] + " " + strconv.Itoa(value.Year())
}

func queryPage(r *http.Request) int { page, _ := strconv.Atoi(r.URL.Query().Get("page")); if page < 1 { return 1 }; if page > 100000 { return 100000 }; return page }
func pageQuery(page int) string { if page <= 1 { return "" }; return "?page=" + strconv.Itoa(page) }

type sitemapURL struct { Location string `xml:"loc"`; LastModified string `xml:"lastmod,omitempty"`; ChangeFreq string `xml:"changefreq,omitempty"` }
type sitemapDocument struct { XMLName xml.Name `xml:"urlset"`; XMLNS string `xml:"xmlns,attr"`; URLs []sitemapURL `xml:"url"` }

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	items := make([]sitemapURL, 0, 256)
	add := func(path, last string) { items = append(items, sitemapURL{Location: s.cfg.SiteURL + path, LastModified: last, ChangeFreq: "weekly"}) }
	add("/", time.Now().UTC().Format("2006-01-02"))
	for _, cluster := range s.clusterList() { add("/cluster/"+url.PathEscape(cluster.Slug), "") }
	if s.db != nil {
		rows, err := s.db.QueryContext(r.Context(), `SELECT id,created_at FROM jokes ORDER BY id`)
		if err == nil { for rows.Next() { var id int64; var created time.Time; if rows.Scan(&id,&created) == nil { add("/joke/"+strconv.FormatInt(id,10), created.UTC().Format("2006-01-02")) } }; rows.Close() }
		userRows, err := s.db.QueryContext(r.Context(), `SELECT id,created_at FROM users ORDER BY id`)
		if err == nil { for userRows.Next() { var id int64; var created time.Time; if userRows.Scan(&id,&created) == nil { add("/user/"+strconv.FormatInt(id,10), created.UTC().Format("2006-01-02")) } }; userRows.Close() }
	} else {
		s.mu.RLock()
		for id, joke := range s.jokes { add("/joke/"+strconv.FormatInt(id,10), joke.CreatedAt.UTC().Format("2006-01-02")) }
		for id, user := range s.users { add("/user/"+strconv.FormatInt(id,10), user.CreatedAt.UTC().Format("2006-01-02")) }
		s.mu.RUnlock()
	}
	for _, tag := range s.popularTagList(1000000) { add("/tag/"+url.PathEscape(tag), "") }
	encoded, err := xml.MarshalIndent(sitemapDocument{XMLNS: "https://www.sitemaps.org/schemas/sitemap/0.9", URLs: items}, "", "  ")
	if err != nil { http.Error(w, "Не удалось создать карту сайта.", http.StatusInternalServerError); return }
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(append([]byte(xml.Header), encoded...))
}

func (s *Server) popularTagList(limit int) []string {
	counts := map[string]int{}
	s.mu.RLock(); for _, joke := range s.jokes { for _, tag := range joke.Triggers { counts[tag]++ } }; s.mu.RUnlock()
	tags := make([]string, 0, len(counts)); for tag := range counts { tags = append(tags, tag) }
	// A modest deterministic cap prevents a pathological tag list in the sitemap.
	if len(tags) > limit { tags = tags[:limit] }
	return tags
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte("User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /auth/\nDisallow: /admin\nSitemap: " + s.cfg.SiteURL + "/sitemap.xml\n"))
}
