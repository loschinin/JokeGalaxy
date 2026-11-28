package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/loschinin/JokeGalaxy/backend/internal/ai"
)

type Config struct {
	DatabaseURL          string
	Port                 string
	JWTSecret            string
	YandexClientID       string
	YandexClientSecret   string
	YandexFolderID       string
	YandexAPIKey         string
	DeepSeekAPIKey       string
	YooKassaShopID       string
	YooKassaSecretKey    string
	SiteURL              string
	FrontendDist         string
}

type Server struct {
	cfg Config
	db *sql.DB
	ai *ai.Client
	mux *http.ServeMux

	// Embeddings, PCA basis, and map coordinates deliberately live in RAM. All
	// related state is guarded by mu so projections and searches share snapshots.
	mu sync.RWMutex
	pcaMu sync.Mutex
	jokes map[int64]Joke
	vectors map[int64][]float32
	coordinates map[int64][3]float64
	pcaMean []float64
	pcaComponents [3][]float64
	pcaInitialized bool
	pcaDataVersion uint64
	clusters map[string]Cluster
	users map[int64]User
	yandexUsers map[string]int64
	likes map[Relation]bool
	favorites map[Relation]bool
	imports map[string]*ImportStatus
	nextJokeID int64
	nextUserID int64

	reindexCtx context.Context
	cancelReindex context.CancelFunc
	reindexMu sync.Mutex
	reindexWG sync.WaitGroup
	reindexRunning bool
	reindexClosed bool
}

func NewFromEnv() (*Server, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	siteURL := strings.TrimRight(os.Getenv("SITE_URL"), "/")
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		if len(jwtSecret) < 32 { return nil, fmt.Errorf("JWT_SECRET must contain at least 32 characters in production") }
		if !strings.HasPrefix(strings.ToLower(siteURL), "https://") { return nil, fmt.Errorf("SITE_URL must use HTTPS in production") }
	}
	config := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port: os.Getenv("PORT"),
		JWTSecret: jwtSecret,
		YandexClientID: os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret: os.Getenv("YANDEX_CLIENT_SECRET"),
		YandexFolderID: os.Getenv("YANDEX_FOLDER_ID"),
		YandexAPIKey: os.Getenv("YANDEX_API_KEY"),
		DeepSeekAPIKey: os.Getenv("DEEPSEEK_API_KEY"),
		YooKassaShopID: os.Getenv("YOOKASSA_SHOP_ID"),
		YooKassaSecretKey: os.Getenv("YOOKASSA_SECRET_KEY"),
		SiteURL: siteURL,
		FrontendDist: os.Getenv("FRONTEND_DIST"),
	}
	return New(config)
}

func New(config Config) (*Server, error) {
	if config.JWTSecret == "" { config.JWTSecret = "development-secret-change-before-deploy" }
	if config.SiteURL == "" { config.SiteURL = "http://localhost:8080" }
	if config.FrontendDist == "" { config.FrontendDist = "../frontend/dist" }
	reindexCtx, cancelReindex := context.WithCancel(context.Background())
	server := &Server{
		cfg: config,
		ai: ai.New(config.DeepSeekAPIKey, config.YandexAPIKey, config.YandexFolderID),
		reindexCtx: reindexCtx, cancelReindex: cancelReindex,
		jokes: make(map[int64]Joke), vectors: make(map[int64][]float32), coordinates: make(map[int64][3]float64),
		clusters: make(map[string]Cluster), users: make(map[int64]User), yandexUsers: make(map[string]int64),
		likes: make(map[Relation]bool), favorites: make(map[Relation]bool), imports: make(map[string]*ImportStatus),
		nextJokeID: 1, nextUserID: 1,
	}
	server.initializeClusters()

	if strings.TrimSpace(config.DatabaseURL) != "" {
		db, err := sql.Open("pgx", config.DatabaseURL)
		if err != nil {
			log.Printf("PostgreSQL disabled; using empty in-memory store: %v", err)
		} else {
			db.SetMaxOpenConns(12)
			db.SetMaxIdleConns(4)
			db.SetConnMaxLifetime(30 * time.Minute)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = db.PingContext(ctx)
			cancel()
			if err != nil {
				log.Printf("PostgreSQL unavailable; using empty in-memory store: %v", err)
				_ = db.Close()
			} else {
				server.db = db
				if err := server.runMigrations(); err != nil { log.Printf("migration warning: %v", err) }
				if err := server.loadDatabaseCache(); err != nil { log.Printf("database cache warning: %v", err) }
			}
		}
	}
	if server.needs3DRecompute() {
		pcaCtx, cancelPCA := context.WithTimeout(context.Background(), 5*time.Minute)
		if _, err := server.recompute3DCoordinates(pcaCtx); err != nil {
			log.Printf("3D PCA startup recompute failed: %v", err)
		}
		cancelPCA()
	}
	server.startMissingEmbeddingReindex()
	server.mux = server.routes()
	return server, nil
}

func (s *Server) Close() {
	s.reindexMu.Lock()
	s.reindexClosed = true
	if s.cancelReindex != nil { s.cancelReindex() }
	s.reindexMu.Unlock()
	s.reindexWG.Wait()
	if s.db != nil { _ = s.db.Close() }
}

func (s *Server) runMigrations() error {
	_, source, _, ok := runtime.Caller(0)
	candidates := []string{}
	if ok { candidates = append(candidates, filepath.Join(filepath.Dir(source), "..", "..", "migrations", "001_init.sql")) }
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "backend", "migrations", "001_init.sql"), filepath.Join(cwd, "migrations", "001_init.sql"))
	}
	var content []byte
	var readErr error
	for _, path := range candidates {
		content, readErr = os.ReadFile(path)
		if readErr == nil { break }
	}
	if readErr != nil { return readErr }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	for _, statement := range splitSQLStatements(string(content)) {
		if _, err := tx.ExecContext(ctx, statement); err != nil { _ = tx.Rollback(); return err }
	}
	return tx.Commit()
}

func splitSQLStatements(source string) []string {
	statements := make([]string, 0, 16)
	start, index := 0, 0
	state, dollarTag := byte(0), ""
	for index < len(source) {
		current := source[index]
		switch state {
		case '\'', '"':
			if current == state {
				if index+1 < len(source) && source[index+1] == state { index += 2; continue }
				state = 0
			}
			index++
		case '-':
			if current == '\n' { state = 0 }
			index++
		case '/':
			if current == '*' && index+1 < len(source) && source[index+1] == '/' { state = 0; index += 2 } else { index++ }
		case '$':
			if strings.HasPrefix(source[index:], dollarTag) { index += len(dollarTag); state = 0 } else { index++ }
		default:
			if current == '\'' || current == '"' { state = current; index++; continue }
			if current == '-' && index+1 < len(source) && source[index+1] == '-' { state = '-'; index += 2; continue }
			if current == '/' && index+1 < len(source) && source[index+1] == '*' { state = '/'; index += 2; continue }
			if current == '$' {
				end := index + 1
				for end < len(source) && (source[end] == '_' || source[end] >= 'a' && source[end] <= 'z' || source[end] >= 'A' && source[end] <= 'Z' || source[end] >= '0' && source[end] <= '9') { end++ }
				if end < len(source) && source[end] == '$' { dollarTag = source[index:end+1]; state = '$'; index = end+1; continue }
			}
			if current == ';' { if statement := strings.TrimSpace(source[start:index]); statement != "" { statements = append(statements, statement) }; start = index+1 }
			index++
		}
	}
	if statement := strings.TrimSpace(source[start:]); statement != "" { statements = append(statements, statement) }
	return statements
}

func (s *Server) loadDatabaseCache() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, jokeSelect+` ORDER BY j.id ASC`)
	if err != nil { return err }
	defer rows.Close()
	loaded := make([]Joke, 0, 256)
	for rows.Next() {
		joke, err := scanJoke(rows)
		if err != nil { return err }
		loaded = append(loaded, joke)
	}
	if err := rows.Err(); err != nil { return err }
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jokes = make(map[int64]Joke, len(loaded))
	s.vectors = make(map[int64][]float32, len(loaded))
	s.coordinates = make(map[int64][3]float64, len(loaded))
	s.pcaMean = nil
	s.pcaComponents = [3][]float64{}
	s.pcaInitialized = false
	s.pcaDataVersion++
	for _, joke := range loaded {
		id, _ := strconv.ParseInt(joke.ID, 10, 64)
		if vector, ok := storedSemanticVector(joke); ok {
			s.vectors[id] = vector
		} else {
			// Older versions persisted hash vectors when Yandex embeddings failed.
			// They are not semantic embeddings and must never enter vector ranking.
			joke.Embedding = nil
		}
		s.coordinates[id] = [3]float64{joke.PosX, joke.PosY, joke.PosZ}
		s.jokes[id] = joke
		if id >= s.nextJokeID { s.nextJokeID = id + 1 }
	}
	var maxUser int64
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(max(id), 0) FROM users`).Scan(&maxUser)
	if maxUser >= s.nextUserID { s.nextUserID = maxUser + 1 }
	return nil
}

func (s *Server) hasEmbeddingConfig() bool {
	return s.ai != nil && strings.TrimSpace(s.ai.YandexAPIKey) != "" && strings.TrimSpace(s.ai.YandexFolderID) != ""
}

func (s *Server) hasSemanticVector(id int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return isUsableEmbedding(s.vectors[id])
}

func (s *Server) hasMissingSemanticVectors() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id, joke := range s.jokes {
		if !joke.IsDuplicate && !isUsableEmbedding(s.vectors[id]) { return true }
	}
	return false
}

func (s *Server) nextJokeNeedingEmbedding(skip map[int64]bool) (Joke, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var selected Joke
	var selectedID int64
	found := false
	for id, joke := range s.jokes {
		if joke.IsDuplicate || skip[id] || isUsableEmbedding(s.vectors[id]) { continue }
		if !found || id < selectedID { selected, selectedID, found = joke, id, true }
	}
	return selected, found
}

func (s *Server) startMissingEmbeddingReindex() {
	if !s.hasEmbeddingConfig() || !s.hasMissingSemanticVectors() { return }
	s.reindexMu.Lock()
	if s.reindexClosed || s.reindexRunning {
		s.reindexMu.Unlock()
		return
	}
	s.reindexRunning = true
	s.reindexWG.Add(1)
	s.reindexMu.Unlock()
	go func() {
		defer s.reindexWG.Done()
		defer func() { s.reindexMu.Lock(); s.reindexRunning = false; s.reindexMu.Unlock() }()
		s.reindexMissingEmbeddings()
	}()
}

func (s *Server) reindexMissingEmbeddings() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	skipped := make(map[int64]bool)
	processed := 0
	for {
		joke, ok := s.nextJokeNeedingEmbedding(skipped)
		if !ok { return }
		select {
		case <-s.reindexCtx.Done(): return
		case <-ticker.C:
		}
		ctx, cancel := context.WithTimeout(s.reindexCtx, 45*time.Second)
		vector, err := s.ai.Embed(ctx, embeddingText(joke.Text, joke.Triggers), false)
		cancel()
		if err != nil || !isUsableEmbedding(vector) {
			if err == nil { err = fmt.Errorf("embedding response was empty or invalid") }
			log.Printf("semantic embedding backfill paused after %d records: %v", processed, err)
			return
		}
		embedding := embeddingBytes(vector)
		position := s.placeCoordinates(vector)
		if s.db != nil {
			// Only replace the vector and coordinates for the unchanged joke; never alter its text or delete the row.
			result, err := s.db.ExecContext(s.reindexCtx, `UPDATE jokes SET embedding=$1,pos_x=$2,pos_y=$3,pos_z=$4 WHERE id=$5 AND text=$6 AND triggers=$7`, embedding, position[0], position[1], position[2], parseID(joke.ID), joke.Text, joke.Triggers)
			if err != nil {
				log.Printf("semantic embedding backfill paused after %d records: %v", processed, err)
				return
			}
			updated, err := result.RowsAffected()
			if err != nil {
				log.Printf("semantic embedding backfill paused after %d records: %v", processed, err)
				return
			}
			if updated == 0 {
				skipped[parseID(joke.ID)] = true
				continue
			}
		}
		if !s.cacheReindexedEmbedding(joke, vector, embedding, position) { continue }
		processed++
		if processed%100 == 0 { log.Printf("semantic embedding backfill: %d records reindexed", processed) }
	}
}

func (s *Server) cacheReindexedEmbedding(snapshot Joke, vector []float32, embedding []byte, position [3]float64) bool {
	id := parseID(snapshot.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jokes[id]
	if !ok || current.Text != snapshot.Text || !sameStrings(current.Triggers, snapshot.Triggers) { return false }
	current.Embedding = embedding
	current.PosX, current.PosY, current.PosZ = position[0], position[1], position[2]
	s.jokes[id] = current
	s.vectors[id] = append([]float32(nil), vector...)
	s.coordinates[id] = position
	s.pcaDataVersion++
	return true
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) { return false }
	for index := range left { if left[index] != right[index] { return false } }
	return true
}

func (s *Server) initializeClusters() {
	clusters := []Cluster{
		{ID: 1, Name: "Каламбуры и игра слов", Slug: "puns", Color: "#FFD700", Description: "Омофоны, двусмысленности, искажённые слова, абсурдные факты."},
		{ID: 2, Name: "IT и программисты", Slug: "it", Color: "#00E5FF", Description: "Разработка, баги, костыли, собеседования, админы, нейросети."},
		{ID: 3, Name: "Отношения, семья и секс", Slug: "relationships", Color: "#FF4081", Description: "Муж и жена, свидания, тёща, любовники, 18+, флирт."},
		{ID: 4, Name: "Врачи и здоровье", Slug: "medicine", Color: "#00E676", Description: "Больница, психиатры, стоматологи, диеты, вредные привычки, аптека."},
		{ID: 5, Name: "Работа, деньги и офис", Slug: "work-money", Color: "#FF9100", Description: "Начальники, зарплата, бедность, кредиты, коллеги, бизнес."},
		{ID: 6, Name: "Политика, общество и законы", Slug: "politics", Color: "#FF5252", Description: "Чиновники, налоги, выборы, ЖКХ, дороги, новости, менталитет."},
		{ID: 7, Name: "Школа, студенты и армия", Slug: "study-army", Color: "#7C4DFF", Description: "Вовочка, учителя, экзамены, ЕГЭ, военкомат, прапорщики."},
		{ID: 8, Name: "Кино, герои и фольклор", Slug: "heroes", Color: "#448AFF", Description: "Штирлиц, Ржевский, сказки, животные, книги, поп-культура."},
		{ID: 9, Name: "Черный юмор и философия", Slug: "dark-philosophy", Color: "#B0BEC5", Description: "Сарказм, цинизм, старость, похороны, экзистенциальная ирония."},
	}
	for _, cluster := range clusters { s.clusters[cluster.Slug] = cluster }
}

func hashEmbedding(text string) []float32 {
	values := make([]float32, embeddingDimensions)
	var norm float64
	for index := 0; index < embeddingDimensions; index++ {
		digest := sha256.Sum256([]byte(strings.ToLower(text) + "\x00" + strconv.Itoa(index)))
		n := binary.LittleEndian.Uint32(digest[:4])
		value := (float64(n)/float64(^uint32(0)))*2 - 1
		values[index] = float32(value)
		norm += value * value
	}
	if norm > 0 {
		divisor := float32(math.Sqrt(norm))
		for index := range values { values[index] /= divisor }
	}
	return values
}

func embeddingBytes(values []float32) []byte {
	if len(values) != embeddingDimensions { return nil }
	result := make([]byte, embeddingDimensions*4)
	for index, value := range values { binary.LittleEndian.PutUint32(result[index*4:], math.Float32bits(value)) }
	return result
}

func vectorFromBytes(data []byte) []float32 {
	if len(data) != embeddingDimensions*4 { return nil }
	values := make([]float32, embeddingDimensions)
	for index := range values { values[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:])) }
	return values
}

func dot(left, right []float32) float64 {
	if len(left) != embeddingDimensions || len(right) != embeddingDimensions { return 0 }
	var total float64
	for index := range left { total += float64(left[index]) * float64(right[index]) }
	return total
}

func (s *Server) cacheJoke(joke Joke) {
	id, _ := strconv.ParseInt(joke.ID, 10, 64)
	vector, hasVector := storedSemanticVector(joke)
	if !hasVector {
		joke.Embedding = nil
		joke.PosX, joke.PosY, joke.PosZ = 0, 0, 0
	}
	position := [3]float64{joke.PosX, joke.PosY, joke.PosZ}
	s.mu.Lock()
	s.jokes[id] = joke
	if hasVector { s.vectors[id] = vector } else { delete(s.vectors, id) }
	s.coordinates[id] = position
	s.pcaDataVersion++
	if id >= s.nextJokeID { s.nextJokeID = id + 1 }
	s.mu.Unlock()
}

func (s *Server) newJokeID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextJokeID
	s.nextJokeID++
	return id
}

func (s *Server) clusterList() []Cluster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Cluster, 0, len(s.clusters))
	for _, cluster := range s.clusters { list = append(list, cluster) }
	for i := 0; i < len(list); i++ { for j := i + 1; j < len(list); j++ { if list[j].ID < list[i].ID { list[i], list[j] = list[j], list[i] } } }
	return list
}
