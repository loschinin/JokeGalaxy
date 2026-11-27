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
	"math/rand"
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

	// Embeddings and map coordinates deliberately live in RAM. All related maps
	// share a read/write lock so vector search never needs per-row DB round trips.
	mu sync.RWMutex
	jokes map[int64]Joke
	vectors map[int64][]float32
	coordinates map[int64][3]float64
	clusters map[string]Cluster
	users map[int64]User
	yandexUsers map[string]int64
	likes map[Relation]bool
	favorites map[Relation]bool
	imports map[string]*ImportStatus
	nextJokeID int64
	nextUserID int64
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
	server := &Server{
		cfg: config,
		ai: ai.New(config.DeepSeekAPIKey, config.YandexAPIKey, config.YandexFolderID),
		jokes: make(map[int64]Joke), vectors: make(map[int64][]float32), coordinates: make(map[int64][3]float64),
		clusters: make(map[string]Cluster), users: make(map[int64]User), yandexUsers: make(map[string]int64),
		likes: make(map[Relation]bool), favorites: make(map[Relation]bool), imports: make(map[string]*ImportStatus),
		nextJokeID: 1, nextUserID: 1,
	}
	server.seedMemory()

	if strings.TrimSpace(config.DatabaseURL) != "" {
		db, err := sql.Open("pgx", config.DatabaseURL)
		if err != nil {
			log.Printf("PostgreSQL disabled; using RAM demo store: %v", err)
		} else {
			db.SetMaxOpenConns(12)
			db.SetMaxIdleConns(4)
			db.SetConnMaxLifetime(30 * time.Minute)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = db.PingContext(ctx)
			cancel()
			if err != nil {
				log.Printf("PostgreSQL unavailable; using RAM demo store: %v", err)
				_ = db.Close()
			} else {
				server.db = db
				if err := server.runMigrations(); err != nil { log.Printf("migration warning: %v", err) }
				if err := server.seedDatabase(); err != nil { log.Printf("database seed warning: %v", err) }
				if err := server.loadDatabaseCache(); err != nil { log.Printf("database cache warning: %v", err) }
			}
		}
	}
	server.mux = server.routes()
	return server, nil
}

func (s *Server) Close() {
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

func (s *Server) seedDatabase() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM jokes`).Scan(&count); err != nil { return err }
	if count > 0 { return nil }
	var authorID int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO users (username, email, bio) VALUES ($1, $2, $3) RETURNING id`, "Редакция Галактики", "editor@jokegalaxy.local", "Шутки, с которых начинается исследование.").Scan(&authorID)
	if err != nil { return err }
	for _, sample := range sampleSeeds() {
		cluster, ok := s.clusters[sample.cluster]
		if !ok { continue }
		vector := hashEmbedding("Темы: " + strings.Join(sample.triggers, ", ") + ". " + sample.text)
		bytes := embeddingBytes(vector)
		pos := s.placeCoordinates(vector)
		_, err := s.db.ExecContext(ctx, `INSERT INTO jokes (user_id, text, triggers, cluster, pos_x, pos_y, pos_z, embedding, likes_count, favorites_count) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, authorID, sample.text, sample.triggers, cluster.Slug, pos[0], pos[1], pos[2], bytes, sample.likes, sample.likes/7)
		if err != nil { log.Printf("seed joke: %v", err) }
	}
	return nil
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
	for _, joke := range loaded {
		id, _ := strconv.ParseInt(joke.ID, 10, 64)
		if len(joke.Embedding) == embeddingDimensions*4 { s.vectors[id] = vectorFromBytes(joke.Embedding) } else { s.vectors[id] = hashEmbedding("Темы: " + strings.Join(joke.Triggers, ", ") + ". " + joke.Text) }
		s.coordinates[id] = [3]float64{joke.PosX, joke.PosY, joke.PosZ}
		s.jokes[id] = joke
		if id >= s.nextJokeID { s.nextJokeID = id + 1 }
	}
	var maxUser int64
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(max(id), 0) FROM users`).Scan(&maxUser)
	if maxUser >= s.nextUserID { s.nextUserID = maxUser + 1 }
	return nil
}

func (s *Server) seedMemory() {
	defaults := []Cluster{
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
	for _, cluster := range defaults { s.clusters[cluster.Slug] = cluster }
	seedAuthor := User{ID: 1, Username: "Редакция Галактики", Email: "editor@jokegalaxy.local", Bio: "Первые орбиты коллекции.", Role: "user", StandupCredits: 50, CreatedAt: time.Now()}
	s.users[seedAuthor.ID] = seedAuthor
	s.nextUserID = 2
	for index, sample := range sampleSeeds() {
		cluster := s.clusters[sample.cluster]
		id := int64(index + 1)
		angle := float64(index) * 2.399963
		radius := .28 + float64(index%7)*.105
		vector := hashEmbedding("Темы: " + strings.Join(sample.triggers, ", ") + ". " + sample.text)
		joke := Joke{
			ID: strconv.FormatInt(id, 10), Text: sample.text, Triggers: append([]string(nil), sample.triggers...), Cluster: cluster,
			Author: Author{ID: "1", Username: seedAuthor.Username, Bio: seedAuthor.Bio}, LikesCount: sample.likes, FavoritesCount: sample.likes / 7,
			CreatedAt: time.Now().Add(-time.Duration(index) * 7 * time.Hour), PosX: math.Cos(angle)*radius, PosY: math.Sin(angle)*radius*.72, PosZ: math.Sin(angle*.56)*radius,
			Embedding: embeddingBytes(vector), UserID: 1,
		}
		s.jokes[id] = joke
		s.vectors[id] = vector
		s.coordinates[id] = [3]float64{joke.PosX, joke.PosY, joke.PosZ}
	}
	s.nextJokeID = int64(len(s.jokes) + 1)
}

type seedJoke struct { text string; cluster string; triggers []string; likes int }

func sampleSeeds() []seedJoke {
	return []seedJoke{
		{"— Почему программист не любит природу?\n— Там слишком много багов, а исправлять их некому.", "it", []string{"программист", "баги", "природа", "работа"}, 148},
		{"На собеседовании спросили, умею ли я работать в команде.\nСказал: «Конечно. Я уже десять лет убеждаю себя, что мы справимся».", "work-money", []string{"собеседование", "команда", "офис", "самоирония"}, 97},
		{"— Ты меня слушаешь?\n— Конечно. Просто у меня лицо загружается отдельно от мыслей.", "relationships", []string{"отношения", "разговор", "внимание", "семья"}, 84},
		{"Врач сказал: «Вам нужен покой».\nТеперь я не отвечаю даже на сообщения от банка. Лечусь комплексно.", "medicine", []string{"врач", "покой", "здоровье", "банк"}, 126},
		{"Купил ежедневник для планирования бюджета.\nТеперь я точно знаю, в какой день у меня закончатся деньги.", "work-money", []string{"деньги", "бюджет", "планирование", "зарплата"}, 113},
		{"— Почему календарь такой спокойный?\n— У него все дни расписаны.", "puns", []string{"календарь", "расписание", "двусмысленность", "слова"}, 204},
		{"Учительница: «Кто назовёт два местоимения?»\nВовочка: «Кто, я?» — «Молодец, оба правильно!»", "study-army", []string{"школа", "местоимение", "Вовочка", "урок"}, 102},
		{"Штирлиц долго смотрел на карту. Карта смотрела в ответ.\nОба понимали: маршрут домой опять построен через ремонт.", "heroes", []string{"Штирлиц", "карта", "ремонт", "кино"}, 76},
		{"Оптимист видит свет в конце тоннеля.\nПессимист — поезд. Реалист — расписание задержек.", "dark-philosophy", []string{"оптимист", "тоннель", "поезд", "ирония"}, 165},
		{"— Как понять, что сервер устал?\n— Он начинает отвечать: «Я подумаю над этим после перезагрузки».", "it", []string{"сервер", "усталость", "перезагрузка", "технологии"}, 92},
		{"В аптеке спросили: «Вам что-нибудь от нервов?»\n«Да. Скажите, что очередь движется быстрее, чем кажется».", "medicine", []string{"аптека", "очередь", "нервы", "ожидание"}, 68},
		{"В семье главное — уметь уступать.\nНапример, я уступил жене право выбирать фильм. Уже третий раз пересматриваем её выбор.", "relationships", []string{"семья", "фильм", "компромисс", "вечер"}, 88},
		{"Начальник сказал: «Нам нужна команда, которая горит работой».\nПосле этих слов офис проверили на пожарную безопасность.", "work-money", []string{"начальник", "команда", "офис", "выгорание"}, 132},
		{"Налоговая прислала письмо: «Мы вас нашли».\nЯ ответил: «А я вас и не терял».", "politics", []string{"налоги", "письмо", "государство", "переписка"}, 119},
		{"Студент так долго готовился к экзамену, что в итоге сдал не предмет, а надежду.", "study-army", []string{"студент", "экзамен", "подготовка", "учёба"}, 109},
		{"Дракон охранял сокровища так надёжно, что сам не мог вспомнить пароль от пещеры.", "heroes", []string{"дракон", "сказка", "пароль", "сокровища"}, 81},
		{"— Ты уже взрослый?\n— По документам да. По ощущениям — всё ещё жду, когда придёт взрослый и всё объяснит.", "dark-philosophy", []string{"взрослая жизнь", "документы", "ожидание", "самоирония"}, 157},
		{"Программист поставил будильник на пять минут раньше.\nТеперь он опаздывает с запасом.", "it", []string{"будильник", "программист", "время", "опоздание"}, 73},
		{"Слова «всё под контролем» особенно убедительно звучат, когда их произносит человек с открытым зонтом в помещении.", "puns", []string{"контроль", "зонт", "слова", "абсурд"}, 61},
		{"Психолог спросил, умею ли я говорить «нет».\nЯ ответил: «Да, но только если вы не против».", "medicine", []string{"психолог", "границы", "нет", "вежливость"}, 95},
		{"Премия — это когда зарплата на секунду заходит в гости и оставляет чаевые.", "work-money", []string{"премия", "зарплата", "деньги", "офис"}, 141},
		{"В очереди за справкой я познакомился с человеком, который пришёл вчера.\nТеперь у меня есть друг и пример для подражания.", "politics", []string{"очередь", "справка", "ожидание", "бюрократия"}, 64},
		{"На контрольной учитель сказал: «Работаем самостоятельно».\nКласс впервые за год понял, что это не название предмета.", "study-army", []string{"контрольная", "учитель", "самостоятельность", "школа"}, 93},
		{"Смотрю фильм про путешествия во времени.\nОчень удобно: если сюжет не понравится, перемотаю до момента, когда ещё не включил.", "heroes", []string{"кино", "время", "сюжет", "путешествие"}, 117},
		{"Жизнь — это длинный список дел, который почему-то не заканчивается после того, как ты вычеркнул слово «жизнь».", "dark-philosophy", []string{"жизнь", "список дел", "философия", "ирония"}, 188},
		{"Умный дом сам заказал фильтр для воды.\nПохоже, теперь в семье только он умеет заботиться о будущем.", "it", []string{"умный дом", "покупки", "технологии", "семья"}, 86},
		{"— Почему у слова «коротко» столько букв?\n— Чтобы оно успело высказаться.", "puns", []string{"слова", "буквы", "коротко", "каламбур"}, 224},
		{"На свидании я сказал: «Я люблю спонтанность».\nМы спонтанно обсудили, почему оба выбрали ресторан по отзывам.", "relationships", []string{"свидание", "спонтанность", "ресторан", "отношения"}, 91},
	}
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

func (s *Server) placeCoordinates(vector []float32) [3]float64 {
	type neighbor struct { score float64; point [3]float64 }
	s.mu.RLock()
	neighbors := make([]neighbor, 0, 5)
	for id, cached := range s.vectors {
		position, ok := s.coordinates[id]
		if !ok { continue }
		candidate := neighbor{score: dot(vector, cached), point: position}
		neighbors = append(neighbors, candidate)
	}
	s.mu.RUnlock()
	if len(neighbors) > 5 {
		for i := 0; i < 5; i++ {
			best := i
			for j := i + 1; j < len(neighbors); j++ { if neighbors[j].score > neighbors[best].score { best = j } }
			neighbors[i], neighbors[best] = neighbors[best], neighbors[i]
		}
		neighbors = neighbors[:5]
	}
	var point [3]float64
	var weightSum float64
	for _, item := range neighbors {
		weight := math.Pow(math.Max(item.score, .04), 6)
		for axis := range point { point[axis] += item.point[axis] * weight }
		weightSum += weight
	}
	if weightSum > 0 { for axis := range point { point[axis] /= weightSum } }
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for axis := range point { point[axis] += (rng.Float64() - .5) * .035 }
	return point
}

func (s *Server) cacheJoke(joke Joke) {
	id, _ := strconv.ParseInt(joke.ID, 10, 64)
	vector := vectorFromBytes(joke.Embedding)
	if vector == nil { vector = hashEmbedding("Темы: " + strings.Join(joke.Triggers, ", ") + ". " + joke.Text) }
	s.mu.Lock()
	s.jokes[id] = joke
	s.vectors[id] = vector
	s.coordinates[id] = [3]float64{joke.PosX, joke.PosY, joke.PosZ}
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
