package services

import (
	"encoding/json"
	"sync"

	"github.com/drama-generator/backend/pkg/ai"
)

type AIServiceSettings struct {
	ConcurrencyEnabled *bool `json:"concurrency_enabled"`
	MaxConcurrency     *int  `json:"max_concurrency"`
}

func parseAIServiceSettings(raw string) (AIServiceSettings, error) {
	if raw == "" {
		return AIServiceSettings{}, nil
	}

	var settings AIServiceSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return AIServiceSettings{}, err
	}

	return settings, nil
}

func (s AIServiceSettings) concurrencyLimit() int {
	if s.ConcurrencyEnabled != nil && !*s.ConcurrencyEnabled {
		return 1
	}
	if s.ConcurrencyEnabled != nil {
		return 0
	}
	if s.MaxConcurrency != nil {
		if *s.MaxConcurrency <= 0 {
			return 0
		}
		return *s.MaxConcurrency
	}
	return 0
}

type aiConcurrencyLimiter struct {
	limit int
	ch    chan struct{}
}

func newAIConcurrencyLimiter(limit int) *aiConcurrencyLimiter {
	return &aiConcurrencyLimiter{
		limit: limit,
		ch:    make(chan struct{}, limit),
	}
}

func (l *aiConcurrencyLimiter) acquire() func() {
	l.ch <- struct{}{}
	return func() {
		<-l.ch
	}
}

var aiConcurrencyLimiters = struct {
	mu      sync.Mutex
	byConfig map[uint]*aiConcurrencyLimiter
}{
	byConfig: make(map[uint]*aiConcurrencyLimiter),
}

func getLimiterForConfig(configID uint, limit int) *aiConcurrencyLimiter {
	if limit <= 0 {
		return nil
	}

	aiConcurrencyLimiters.mu.Lock()
	defer aiConcurrencyLimiters.mu.Unlock()

	if existing, ok := aiConcurrencyLimiters.byConfig[configID]; ok && existing.limit == limit {
		return existing
	}

	limiter := newAIConcurrencyLimiter(limit)
	aiConcurrencyLimiters.byConfig[configID] = limiter
	return limiter
}

type limitedAIClient struct {
	client  ai.AIClient
	limiter *aiConcurrencyLimiter
}

func wrapAIClientWithLimiter(client ai.AIClient, limiter *aiConcurrencyLimiter) ai.AIClient {
	if limiter == nil {
		return client
	}
	return &limitedAIClient{
		client:  client,
		limiter: limiter,
	}
}

func (c *limitedAIClient) GenerateText(prompt string, systemPrompt string, options ...func(*ai.ChatCompletionRequest)) (string, error) {
	release := c.limiter.acquire()
	defer release()
	return c.client.GenerateText(prompt, systemPrompt, options...)
}

func (c *limitedAIClient) GenerateImage(prompt string, size string, n int) ([]string, error) {
	release := c.limiter.acquire()
	defer release()
	return c.client.GenerateImage(prompt, size, n)
}

func (c *limitedAIClient) TestConnection() error {
	release := c.limiter.acquire()
	defer release()
	return c.client.TestConnection()
}
