package app

import "time"

const embeddingDimensions = 256

type Cluster struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type Author struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Bio       string `json:"bio,omitempty"`
}

type Joke struct {
	ID              string    `json:"id"`
	Text            string    `json:"text"`
	Triggers        []string  `json:"triggers"`
	Cluster         Cluster   `json:"cluster"`
	Author          Author    `json:"author"`
	LikesCount      int       `json:"likesCount"`
	FavoritesCount  int       `json:"favoritesCount"`
	CreatedAt       time.Time `json:"createdAt"`
	PosX            float64   `json:"posX"`
	PosY            float64   `json:"posY"`
	PosZ            float64   `json:"posZ"`
	Liked           bool      `json:"liked,omitempty"`
	Favorited       bool      `json:"favorited,omitempty"`
	Relevance       float64   `json:"relevance,omitempty"`
	Embedding       []byte    `json:"-"`
	UserID          int64     `json:"-"`
	IsDuplicate     bool      `json:"isDuplicate,omitempty"`
}

type User struct {
	ID                  int64     `json:"id"`
	YandexID            string    `json:"yandex_id,omitempty"`
	Email               string    `json:"email,omitempty"`
	Username            string    `json:"username"`
	AvatarURL           string    `json:"avatar_url,omitempty"`
	Bio                 string    `json:"bio,omitempty"`
	Role                string    `json:"role"`
	StandupCredits      int       `json:"standup_credits"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"created_at"`
}

type SearchResponse struct {
	KeywordResults  []Joke `json:"keyword_results"`
	SemanticResults []Joke `json:"semantic_results"`
	Results         []Joke `json:"results"`
	Query           string `json:"query"`
	Mode            string `json:"mode"`
}

type JokeInput struct {
	Text     string   `json:"text"`
	Cluster  string   `json:"cluster"`
	Triggers []string `json:"triggers"`
}

type Relation struct {
	UserID int64
	JokeID int64
}

type ImportStatus struct {
	ID           string    `json:"job_id"`
	Filename     string    `json:"filename"`
	Status       string    `json:"status"`
	TotalRows    int       `json:"total_rows"`
	Processed    int       `json:"processed_rows"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
