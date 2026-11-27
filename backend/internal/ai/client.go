package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	deepSeekEndpoint = "https://api.deepseek.com/chat/completions"
	yandexEmbeddingEndpoint = "https://llm.api.cloud.yandex.net/foundationModels/v1/textEmbedding"
)

type Client struct {
	HTTP              *http.Client
	DeepSeekAPIKey    string
	YandexAPIKey      string
	YandexFolderID    string
}

type Analysis struct {
	Triggers []string `json:"triggers"`
	Cluster  string   `json:"cluster"`
	Themes   []string `json:"themes"`
}

func New(deepSeekKey, yandexKey, folderID string) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 45 * time.Second},
		DeepSeekAPIKey: deepSeekKey,
		YandexAPIKey: yandexKey,
		YandexFolderID: folderID,
	}
}

func (c *Client) Analyze(ctx context.Context, text string, clusters []string) (Analysis, error) {
	if strings.TrimSpace(c.DeepSeekAPIKey) == "" {
		return Analysis{}, errors.New("DEEPSEEK_API_KEY is not configured")
	}
	clusterList := strings.Join(clusters, ", ")
	prompt := fmt.Sprintf("Проанализируй русскую шутку. Верни только JSON: {\"triggers\":[5-8 коротких русских смысловых тегов],\"cluster\":\"slug\",\"themes\":[2-3 темы]}. Выбери cluster только из: %s. Текст: %q", clusterList, text)
	content, err := c.chat(ctx, []message{{Role: "system", Content: "Ты редактор русскоязычного юмористического каталога. Отвечай кратко и строго по схеме JSON."}, {Role: "user", Content: prompt}}, false)
	if err != nil { return Analysis{}, err }
	var result Analysis
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// DeepSeek occasionally wraps JSON in a Markdown fence; extract its object.
		start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
		if start < 0 || end <= start { return Analysis{}, fmt.Errorf("decode AI analysis: %w", err) }
		if decodeErr := json.Unmarshal([]byte(content[start:end+1]), &result); decodeErr != nil { return Analysis{}, decodeErr }
	}
	return result, nil
}

func (c *Client) ExtractThemes(ctx context.Context, story string) ([]string, error) {
	if strings.TrimSpace(c.DeepSeekAPIKey) == "" { return nil, errors.New("DEEPSEEK_API_KEY is not configured") }
	prompt := "Извлеки из личной истории 2 или 3 короткие русские комедийные темы. Верни JSON-массив строк без пояснений. История: " + story
	content, err := c.chat(ctx, []message{{Role: "system", Content: "Ты редактор стендап-историй. Выделяй темы, не пересказывай историю."}, {Role: "user", Content: prompt}}, false)
	if err != nil { return nil, err }
	var themes []string
	if err := json.Unmarshal([]byte(content), &themes); err == nil { return limitStrings(themes, 3), nil }
	var wrapped struct { Themes []string `json:"themes"` }
	if err := json.Unmarshal([]byte(content), &wrapped); err == nil { return limitStrings(wrapped.Themes, 3), nil }
	return nil, fmt.Errorf("decode themes: %s", content)
}

func (c *Client) Embed(ctx context.Context, text string, query bool) ([]float32, error) {
	if strings.TrimSpace(c.YandexAPIKey) == "" || strings.TrimSpace(c.YandexFolderID) == "" {
		return nil, errors.New("YANDEX_API_KEY and YANDEX_FOLDER_ID are required")
	}
	model := "text-search-doc"
	if query { model = "text-search-query" }
	payload := map[string]any{
		"modelUri": fmt.Sprintf("emb://%s/%s/latest", c.YandexFolderID, model),
		"text": text,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, yandexEmbeddingEndpoint, bytes.NewReader(body))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Api-Key "+c.YandexAPIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("Yandex embedding returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var result struct { Embedding []float32 `json:"embedding"` }
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil { return nil, err }
	if len(result.Embedding) != 256 { return nil, fmt.Errorf("expected 256 embedding dimensions, got %d", len(result.Embedding)) }
	return normalize(result.Embedding), nil
}

func (c *Client) StreamMonologue(ctx context.Context, story, style string, jokes []string, onChunk func(string) error) error {
	if strings.TrimSpace(c.DeepSeekAPIKey) == "" { return errors.New("DEEPSEEK_API_KEY is not configured") }
	stylePrompt := map[string]string{
		"custom": "Сохраняй естественный голос рассказчика, чётко выстрой сетап, развитие и панчлайн.",
		"carlin": "Используй резкую социальную сатиру, разбирай абсурд правил и слов, наращивай ритм.",
		"zhvanetsky": "Используй тонкую самоиронию, краткие афористичные фразы и бытовые парадоксы.",
		"carter": "Строй материал как Comedy Bible: личная уязвимость, эмоция «что меня бесит или пугает» и выразительные акт-аут панчлайны.",
	}
	if stylePrompt[style] == "" { style = "custom" }
	prompt := fmt.Sprintf("Создай оригинальный русскоязычный стендап-монолог на 350–550 слов по личной истории. %s Используй выбранные шутки только как материал, не вставляй их механически. Сохраняй доброжелательность, не выдумывай факты о рассказчике. История: %s\nПодобранные шутки:\n%s", stylePrompt[style], story, strings.Join(jokes, "\n---\n"))
	return c.streamChat(ctx, []message{{Role: "system", Content: "Ты опытный редактор авторского стендапа на русском языке. Пиши живо и естественно; не добавляй заголовки, если это не нужно для ритма."}, {Role: "user", Content: prompt}}, onChunk)
}

type message struct { Role string `json:"role"`; Content string `json:"content"` }

type chatRequest struct {
	Model string `json:"model"`
	Messages []message `json:"messages"`
	Stream bool `json:"stream"`
	Temperature float64 `json:"temperature,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

func (c *Client) chat(ctx context.Context, messages []message, stream bool) (string, error) {
	payload := chatRequest{Model: "deepseek-chat", Messages: messages, Stream: stream, Temperature: 0.7}
	payload.ResponseFormat = map[string]string{"type": "json_object"}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekEndpoint, bytes.NewReader(body))
	if err != nil { return "", err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.DeepSeekAPIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("DeepSeek returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var result struct { Choices []struct { Message message `json:"message"` } `json:"choices"` }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil { return "", err }
	if len(result.Choices) == 0 { return "", errors.New("DeepSeek returned an empty completion") }
	return result.Choices[0].Message.Content, nil
}

func (c *Client) streamChat(ctx context.Context, messages []message, onChunk func(string) error) error {
	payload := chatRequest{Model: "deepseek-chat", Messages: messages, Stream: true, Temperature: 0.8}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekEndpoint, bytes.NewReader(body))
	if err != nil { return err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.DeepSeekAPIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("DeepSeek returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") { continue }
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" { return nil }
		var event struct { Choices []struct { Delta struct { Content string `json:"content"` } `json:"delta"` } `json:"choices"` }
		if json.Unmarshal([]byte(data), &event) != nil || len(event.Choices) == 0 { continue }
		chunk := event.Choices[0].Delta.Content
		if chunk != "" { if err := onChunk(chunk); err != nil { return err } }
	}
	return scanner.Err()
}

func limitStrings(values []string, max int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, max)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] { continue }
		seen[strings.ToLower(value)] = true
		out = append(out, value)
		if len(out) == max { break }
	}
	return out
}

func normalize(values []float32) []float32 {
	var norm float64
	for _, value := range values { norm += float64(value) * float64(value) }
	if norm == 0 { return values }
	denom := float32(1 / math.Sqrt(norm))
	for i := range values { values[i] *= denom }
	return values
}
