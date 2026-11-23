// Package cache wraps Redis as the optional second storage level.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cliente is a nil-safe Redis client: every method on a nil *Cliente is a
// cache miss or a no-op, so the API keeps working when Redis is down.
type Cliente struct {
	rdb *redis.Client
}

// Conectar opens a Redis connection using REDIS_ADDR (default redis:6379)
// and REDIS_PASSWORD, and verifies it with PING.
func Conectar(ctx context.Context) (*Cliente, error) {
	direccion := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if direccion == "" {
		direccion = "redis:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         direccion,
		Password:     os.Getenv("REDIS_PASSWORD"),
		DialTimeout:  2 * time.Second,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("error conectando a Redis: %w", err)
	}

	return &Cliente{rdb: rdb}, nil
}

// Obtener returns the cached bytes for key and whether it was a hit.
func (c *Cliente) Obtener(ctx context.Context, clave string) ([]byte, bool) {
	if c == nil || c.rdb == nil {
		return nil, false
	}

	valor, err := c.rdb.Get(ctx, clave).Bytes()
	if err != nil {
		return nil, false
	}

	return valor, true
}

// Guardar stores value under key with a time to live. Errors are ignored
// on purpose: the cache must never break a request.
func (c *Cliente) Guardar(
	ctx context.Context,
	clave string,
	valor []byte,
	ttl time.Duration,
) {
	if c == nil || c.rdb == nil {
		return
	}

	_ = c.rdb.Set(ctx, clave, valor, ttl).Err()
}

// Cerrar releases the connection.
func (c *Cliente) Cerrar() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// ClavePrediccion builds a stable key from the model inputs, so equal
// inputs share one precomputed prediction.
func ClavePrediccion(campos ...string) string {
	h := sha256.New()
	for _, campo := range campos {
		h.Write([]byte(strings.TrimSpace(campo)))
		h.Write([]byte{0})
	}
	return "pred:v1:" + hex.EncodeToString(h.Sum(nil))
}

// ClaveID builds the key used for predictions read by identifier.
func ClaveID(id string) string {
	return "pred:id:" + id
}
