package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Session struct {
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type AuthManager struct {
	mu           sync.RWMutex
	sessions     map[string]Session
	failedIPs    map[string][]time.Time
	blockedUntil map[string]time.Time
}

func NewAuthManager() *AuthManager {
	return &AuthManager{
		sessions:     make(map[string]Session),
		failedIPs:    make(map[string][]time.Time),
		blockedUntil: make(map[string]time.Time),
	}
}

// GetClientIP 提取真实 IP (仅用于服务端本地防爆破频控，绝不向大屏公开)
func GetClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		parts := strings.Split(xForwardedFor, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// CheckRateLimit 检查 IP 是否因暴力破解被封禁
func (a *AuthManager) CheckRateLimit(ip string) (bool, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	if until, blocked := a.blockedUntil[ip]; blocked {
		if now.Before(until) {
			return false, until.Sub(now)
		}
		delete(a.blockedUntil, ip)
		delete(a.failedIPs, ip)
	}
	return true, 0
}

// RecordFailedAttempt 记录一次密码错误，并在达到 5 次时封禁 15 分钟
func (a *AuthManager) RecordFailedAttempt(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	// 保留近 15 分钟内的失败记录
	var recent []time.Time
	for _, t := range a.failedIPs[ip] {
		if now.Sub(t) < 15*time.Minute {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	a.failedIPs[ip] = recent

	if len(recent) >= 5 {
		a.blockedUntil[ip] = now.Add(15 * time.Minute)
	}
}

// RecordSuccess 登录成功，清除该 IP 的错误记录
func (a *AuthManager) RecordSuccess(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.failedIPs, ip)
	delete(a.blockedUntil, ip)
}

// CreateSession 创建持久 Session
func (a *AuthManager) CreateSession() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	b := make([]byte, 24)
	rand.Read(b)
	token := hex.EncodeToString(b)

	now := time.Now()
	a.sessions[token] = Session{
		Token:     token,
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour), // 7 天有效
	}
	return token
}

// ValidateSession 校验 Session 是否合法
func (a *AuthManager) ValidateSession(token string) bool {
	if token == "" {
		return false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	s, exists := a.sessions[token]
	if !exists {
		return false
	}
	if time.Now().After(s.ExpiresAt) {
		return false
	}
	return true
}

// ExtractTokenFromRequest 从 Cookie 或 Header 中提取 Token
func ExtractTokenFromRequest(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	// 2. 自定义 Header X-Aegis-Token
	if custom := r.Header.Get("X-Aegis-Token"); custom != "" {
		return custom
	}
	// 3. Cookie: aegis_session=<token>
	cookie, err := r.Cookie("aegis_session")
	if err == nil && cookie != nil {
		return cookie.Value
	}
	return ""
}
