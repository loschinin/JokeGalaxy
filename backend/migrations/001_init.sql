-- JokeGalaxy schema. Apply with: psql "$DATABASE_URL" -f backend/migrations/001_init.sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    yandex_id TEXT UNIQUE,
    email TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT 'Космический гость',
    avatar_url TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    standup_credits INTEGER NOT NULL DEFAULT 50 CHECK (standup_credits >= 0),
    onboarding_completed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS clusters (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    color VARCHAR(7) NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS jokes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text TEXT NOT NULL CHECK (length(btrim(text)) >= 3),
    triggers TEXT[] NOT NULL DEFAULT '{}',
    cluster TEXT NOT NULL REFERENCES clusters(slug),
    search_tsv TSVECTOR NOT NULL DEFAULT ''::tsvector,
    pos_x DOUBLE PRECISION NOT NULL DEFAULT 0,
    pos_y DOUBLE PRECISION NOT NULL DEFAULT 0,
    pos_z DOUBLE PRECISION NOT NULL DEFAULT 0,
    embedding BYTEA,
    likes_count INTEGER NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    favorites_count INTEGER NOT NULL DEFAULT 0 CHECK (favorites_count >= 0),
    is_duplicate BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (embedding IS NULL OR octet_length(embedding) = 1024)
);

CREATE INDEX IF NOT EXISTS jokes_cluster_created_idx ON jokes(cluster, created_at DESC);
CREATE INDEX IF NOT EXISTS jokes_user_created_idx ON jokes(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS jokes_created_idx ON jokes(created_at DESC);
CREATE INDEX IF NOT EXISTS jokes_likes_idx ON jokes(likes_count DESC);
CREATE INDEX IF NOT EXISTS jokes_favorites_idx ON jokes(favorites_count DESC);
CREATE INDEX IF NOT EXISTS jokes_search_tsv_gin_idx ON jokes USING GIN(search_tsv);
CREATE INDEX IF NOT EXISTS jokes_text_trgm_gin_idx ON jokes USING GIN(text gin_trgm_ops);

CREATE OR REPLACE FUNCTION jokes_search_tsv_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_tsv :=
        setweight(to_tsvector('russian', COALESCE(NEW.text, '')), 'A') ||
        setweight(to_tsvector('russian', COALESCE(array_to_string(NEW.triggers, ' '), '')), 'B');
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS jokes_search_tsv_trigger ON jokes;
CREATE TRIGGER jokes_search_tsv_trigger
BEFORE INSERT OR UPDATE OF text, triggers ON jokes
FOR EACH ROW EXECUTE FUNCTION jokes_search_tsv_update();

CREATE TABLE IF NOT EXISTS joke_likes (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joke_id BIGINT NOT NULL REFERENCES jokes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, joke_id)
);

CREATE TABLE IF NOT EXISTS joke_favorites (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joke_id BIGINT NOT NULL REFERENCES jokes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, joke_id)
);

CREATE TABLE IF NOT EXISTS payments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    yookassa_id TEXT UNIQUE,
    package_code TEXT NOT NULL,
    credits_amount INTEGER NOT NULL CHECK (credits_amount > 0),
    amount_rub NUMERIC(10, 2) NOT NULL CHECK (amount_rub > 0),
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS payments_user_created_idx ON payments(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS standup_scripts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    original_story TEXT NOT NULL,
    style TEXT NOT NULL,
    used_joke_ids BIGINT[] NOT NULL DEFAULT '{}',
    generated_text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS import_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    filename TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    total_rows INTEGER NOT NULL DEFAULT 0,
    processed_rows INTEGER NOT NULL DEFAULT 0,
    error_text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

INSERT INTO clusters (name, slug, color, description) VALUES
('Каламбуры и игра слов', 'puns', '#FFD700', 'Омофоны, двусмысленности, искажённые слова, абсурдные факты.'),
('IT и программисты', 'it', '#00E5FF', 'Разработка, баги, костыли, собеседования, админы, нейросети.'),
('Отношения, семья и секс', 'relationships', '#FF4081', 'Муж и жена, свидания, тёща, любовники, флирт.'),
('Врачи и здоровье', 'medicine', '#00E676', 'Больница, психиатры, стоматологи, диеты, вредные привычки, аптека.'),
('Работа, деньги и офис', 'work-money', '#FF9100', 'Начальники, зарплата, бедность, кредиты, коллеги, бизнес.'),
('Политика, общество и законы', 'politics', '#FF5252', 'Чиновники, налоги, выборы, ЖКХ, дороги, новости, менталитет.'),
('Школа, студенты и армия', 'study-army', '#7C4DFF', 'Вовочка, учителя, экзамены, ЕГЭ, военкомат, прапорщики.'),
('Кино, герои и фольклор', 'heroes', '#448AFF', 'Штирлиц, Ржевский, сказки, животные, книги, поп-культура.'),
('Черный юмор и философия', 'dark-philosophy', '#B0BEC5', 'Сарказм, цинизм, старость, похороны, экзистенциальная ирония.')
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    color = EXCLUDED.color,
    description = EXCLUDED.description;
