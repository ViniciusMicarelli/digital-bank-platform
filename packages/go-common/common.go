// Package go-common provides shared utilities and types used across all services in the Digital Bank Platform.
// This monorepo structure allows sharing common code between microservices.
package common

// ServiceInfo contains metadata about a service for observability and governance.
type ServiceInfo struct {
	ServiceKey   string                 `json:"service_key"`
	Version      string                 `json:"version"`
	Description  string                 `json:"description"`
	Dependencies []common.ServiceInfo   `json:"dependencies"`
	HealthCheck  string                 `json:"health_check"`
	Priority     int                    `json:"priority"`
	Owner        string                 `json:"owner"`
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	ServiceKey    string                `json:"service_key"`
	Healthy       bool                  `json:"healthy"`
	LastCheck     string                `json:"last_check"`
	Errors        []error               `json:"errors"`
	ResponseTime  time.Duration         `json:"response_time"`
	Dependencies []healthCheckResult   `json:"dependencies"`
}

// ServiceRegistry holds the registry of all services in the platform.
type ServiceRegistry struct {
	Services map[string]ServiceInfo `json:"services"`
}

// Method defines API method metadata for generated client/stub code.
type Method struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	HTTP      string   `json:"http"`
	HTTPVerb  string   `json:"http_verb"`
	Type      string   `json:"type"`
	RateLimit int      `json:"rate_limit"`
	Auth      string   `json:"auth"`
}

// MessageEvent represents a domain event from the event system.
type MessageEvent struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Service      string             `json:"service"`
	CreatedAt    string             `json:"created_at"`
	Content      json.RawMessage    `json:"content"`
	Priority     int                `json:"priority"`
	TTL          time.Duration      `json:"ttl"`
	CorrelationID string             `json:"correlation_id"`
}

// CancellationToken for controlling long-running operations.
type CancellationToken struct {
	Cancelled   chan struct{}      `json:"-"`
	Cancelled   bool               `json:"cancelled"`
}

// Result represents a generic operation result.
type Result[T any] struct {
	Success  bool     `json:"success"`
	Data     T        `json:"data,omitempty"`
	Error    error    `json:"error,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// Timestamp represents a point in time.
type Timestamp time.Time

// Duration represents a time interval.
type Duration time.Duration
