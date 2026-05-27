package httpapi

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	HeaderInternalKey = "X-HotKey-Internal-Key"
	HeaderTenantID    = "X-HotKey-Tenant-ID"
	HeaderIdempotency = "Idempotency-Key"
	ContextKeyTenant  = "internalTenantID"
)

type idempotencyStore struct {
	mu    sync.RWMutex
	items map[string]bool
}

func newIdempotencyStore() *idempotencyStore {
	return &idempotencyStore{items: make(map[string]bool)}
}

func (s *idempotencyStore) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[key]
}

func (s *idempotencyStore) Put(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = true
}

var globalIdempotencyStore = newIdempotencyStore()

func InternalAPIAuthMiddleware(validKey string, defaultTenantID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		providedKey := c.GetHeader(HeaderInternalKey)
		if providedKey == "" || providedKey != validKey {
			writeError(c, http.StatusUnauthorized, "unauthorized", "missing or invalid internal API key")
			c.Abort()
			return
		}

		tenantID := c.GetHeader(HeaderTenantID)
		if tenantID == "" {
			tenantID = defaultTenantID
		}
		if tenantID == "" {
			writeError(c, http.StatusBadRequest, "missing_tenant_id", "X-HotKey-Tenant-ID header is required")
			c.Abort()
			return
		}
		c.Set(ContextKeyTenant, tenantID)

		if idemKey := c.GetHeader(HeaderIdempotency); idemKey != "" {
			storeKey := tenantID + ":" + idemKey
			if globalIdempotencyStore.Has(storeKey) {
				c.Set("idempotentReplay", true)
			} else {
				globalIdempotencyStore.Put(storeKey)
				c.Set("idempotentReplay", false)
			}
		} else {
			c.Set("idempotentReplay", false)
		}

		c.Next()
	}
}

func getTenantID(c *gin.Context) string {
	if v, ok := c.Get(ContextKeyTenant); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func isIdempotentReplay(c *gin.Context) bool {
	if v, ok := c.Get("idempotentReplay"); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
