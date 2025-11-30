package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/gravitl/netmaker-k8s-ops/conf"
	"github.com/gravitl/netmaker/models"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ProxyMode represents the authentication mode for the proxy
type ProxyMode string

const (
	// AuthMode - requests are impersonated using WireGuard peer identity
	AuthMode ProxyMode = "auth"
	// NoAuthMode - requests are proxied without authentication
	NoAuthMode ProxyMode = "noauth"
)

// ProxyConfig holds configuration for the proxy
type ProxyConfig struct {
	Mode              ProxyMode
	ImpersonateUser   string
	ImpersonateGroups []string
}

// UserMapping represents a mapping from IP to user and groups
type UserMapping struct {
	User   string   `json:"user"`
	Groups []string `json:"groups"`
}

// ExternalAPIConfig holds configuration for external API
type ExternalAPIConfig struct {
	ServerDomain string
	APIToken     string
	SyncInterval time.Duration
}

// UserIPMapWithMutex maintains the mapping of IP addresses to users and groups
type UserIPMapWithMutex struct {
	UserIPMap models.UserIPMap
	mutex     sync.RWMutex
}

// NewUserIPMap creates a new UserIPMap
func NewUserIPMap() *UserIPMapWithMutex {
	return &UserIPMapWithMutex{
		UserIPMap: models.UserIPMap{
			Mappings: make(map[string]models.UserMapping),
		},
	}
}

// SetUserMapping sets the user and groups for a given IP
func (uim *UserIPMapWithMutex) SetUserMapping(ip string, user string, groups []string) {
	uim.mutex.Lock()
	defer uim.mutex.Unlock()
	uim.UserIPMap.Mappings[ip] = models.UserMapping{
		User:   user,
		Groups: groups,
	}
}

// GetUserMapping gets the user and groups for a given IP
func (uim *UserIPMapWithMutex) GetUserMapping(ip string) (string, []string, bool) {
	uim.mutex.RLock()
	defer uim.mutex.RUnlock()
	mapping, exists := uim.UserIPMap.Mappings[ip]
	if !exists {
		return "", nil, false
	}
	return mapping.User, mapping.Groups, true
}

// RemoveUserMapping removes the mapping for a given IP
func (uim *UserIPMapWithMutex) RemoveUserMapping(ip string) {
	uim.mutex.Lock()
	defer uim.mutex.Unlock()
	delete(uim.UserIPMap.Mappings, ip)
}

// GetAllMappings returns all current mappings (for debugging)
func (uim *UserIPMapWithMutex) GetAllMappings() map[string]models.UserMapping {
	uim.mutex.RLock()
	defer uim.mutex.RUnlock()
	// Return a copy to avoid race conditions
	result := make(map[string]models.UserMapping)
	for ip, mapping := range uim.UserIPMap.Mappings {
		result[ip] = mapping
	}
	return result
}

// Global user IP mapping instance
var globalUserIPMap = NewUserIPMap()

// SetUserIPMapping sets the user and groups for a given IP (global function)
func SetUserIPMapping(ip string, user string, groups []string) {
	globalUserIPMap.SetUserMapping(ip, user, groups)
}

// GetUserIPMapping gets the user and groups for a given IP (global function)
func GetUserIPMapping(ip string) (string, []string, bool) {
	return globalUserIPMap.GetUserMapping(ip)
}

// RemoveUserIPMapping removes the mapping for a given IP (global function)
func RemoveUserIPMapping(ip string) {
	globalUserIPMap.RemoveUserMapping(ip)
}

// GetAllUserIPMappings returns all current mappings (global function)
func GetAllUserIPMappings() map[string]models.UserMapping {
	return globalUserIPMap.GetAllMappings()
}

// getNMAPIConfig reads external API configuration from environment variables
func getNMAPIConfig() ExternalAPIConfig {
	config := ExternalAPIConfig{
		ServerDomain: os.Getenv("EXTERNAL_API_SERVER_DOMAIN"),
		APIToken:     os.Getenv("EXTERNAL_API_TOKEN"),
		SyncInterval: 30 * time.Second, // Default sync interval (seconds)
	}

	// Override sync interval if set (expects integer seconds)
	if syncIntervalStr := os.Getenv("EXTERNAL_API_SYNC_INTERVAL"); syncIntervalStr != "" {
		if secs, err := strconv.Atoi(syncIntervalStr); err == nil && secs > 0 {
			config.SyncInterval = time.Duration(secs) * time.Second
		}
	}

	return config
}

// fetchUserMappingsFromAPI fetches user mappings from the external API
func fetchUserMappingsFromAPI(config ExternalAPIConfig, zlog logr.Logger) error {
	if config.ServerDomain == "" || config.APIToken == "" {
		zlog.V(1).Info("External API not configured, skipping fetch")
		return nil
	}

	// Build the API URL
	apiURL := fmt.Sprintf("https://%s%s", config.ServerDomain, "/api/users/network_ip")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip TLS verification for external API
			},
		},
	}

	// Create request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization header
	req.Header.Set("Authorization", "Bearer "+config.APIToken)
	req.Header.Set("Content-Type", "application/json")

	// Make the request
	zlog.V(1).Info("Fetching user mappings from external API", "url", apiURL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResponse models.UserIPMap
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Update the global user IP map
	zlog.Info("Updating user mappings from external API", "count", len(apiResponse.Mappings))
	for ip, mapping := range apiResponse.Mappings {
		SetUserIPMapping(ip, mapping.User, mapping.Groups)
		zlog.V(1).Info("Updated user mapping", "ip", ip, "user", mapping.User, "groups", mapping.Groups)
	}

	return nil
}

// startExternalAPISync starts the periodic synchronization with external API
func startExternalAPISync(ctx context.Context, config ExternalAPIConfig, zlog logr.Logger) {
	if config.ServerDomain == "" || config.APIToken == "" {
		zlog.Info("External API not configured, skipping sync")
		return
	}

	zlog.Info("Starting external API sync", "server", config.ServerDomain, "interval", config.SyncInterval)

	ticker := time.NewTicker(config.SyncInterval)
	defer ticker.Stop()

	// Initial fetch
	if err := fetchUserMappingsFromAPI(config, zlog); err != nil {
		zlog.Error(err, "Failed to fetch initial user mappings from external API")
	}

	// Periodic sync
	for {
		select {
		case <-ctx.Done():
			zlog.Info("Stopping external API sync")
			return
		case <-ticker.C:
			if err := fetchUserMappingsFromAPI(config, zlog); err != nil {
				zlog.Error(err, "Failed to sync user mappings from external API")
			}
		}
	}
}

// getProxyConfig reads proxy configuration from environment variables
func getProxyConfig() ProxyConfig {
	config := ProxyConfig{
		Mode: AuthMode, // Default to auth mode
	}

	// Read proxy mode from environment
	mode := os.Getenv("PROXY_MODE")
	switch strings.ToLower(mode) {
	case "noauth":
		config.Mode = NoAuthMode
	case "auth":
		config.Mode = AuthMode
	default:
		// Default to auth mode if not specified or invalid
		config.Mode = AuthMode
	}

	// Read impersonation settings for auth mode
	if config.Mode == AuthMode {
		config.ImpersonateUser = os.Getenv("PROXY_IMPERSONATE_USER")
		if config.ImpersonateUser == "" {
			// Default to using WireGuard peer IP as username
			config.ImpersonateUser = "wireguard-peer"
		}

		// Read groups from environment (comma-separated)
		groupsStr := os.Getenv("PROXY_IMPERSONATE_GROUPS")
		if groupsStr != "" {
			config.ImpersonateGroups = strings.Split(groupsStr, ",")
			// Trim whitespace from group names
			for i, group := range config.ImpersonateGroups {
				config.ImpersonateGroups[i] = strings.TrimSpace(group)
			}
		} else {
			// Default groups for WireGuard peers
			config.ImpersonateGroups = []string{"system:authenticated", "wireguard-peers"}
		}
	}

	return config
}

func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {

		// Get the client IP address
		clientIP := c.ClientIP()

		// Get the current time
		now := time.Now()
		// Log the request
		log.Printf("[%s] %s %s %s", now.Format(time.RFC3339), c.Request.Method, c.Request.URL.Path, clientIP)

		// Proceed to the next handler
		c.Next()
	}
}

func StartK8sProxy(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	zlog := zap.New(zap.UseDevMode(true))

	// Get proxy configuration
	proxyConfig := getProxyConfig()
	zlog.Info("Proxy configuration", "mode", proxyConfig.Mode, "impersonate_user", proxyConfig.ImpersonateUser, "impersonate_groups", proxyConfig.ImpersonateGroups)

	// Get external API configuration and start sync
	externalAPIConfig := getNMAPIConfig()
	zlog.Info("External API configuration", "server", externalAPIConfig.ServerDomain, "sync_interval", externalAPIConfig.SyncInterval)

	// Start external API sync in background
	go startExternalAPISync(ctx, externalAPIConfig, zlog)

	// Note: Netclient runs as an init container to establish WireGuard connection first
	// Wait a bit for WireGuard interface to be fully established
	zlog.Info("Proxy starting - waiting for WireGuard interface from init container")
	time.Sleep(5 * time.Second)

	// Get Kubernetes configuration
	var config *rest.Config
	var err error
	if conf.InClusterCfg() {
		// Get in-cluster Kubernetes configuration
		config, err = rest.InClusterConfig()
		zlog.Info("Using in-cluster configuration")
	} else {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		zlog.Info("Using kubeconfig", "path", kubeconfigPath)
	}
	if err != nil {
		zlog.Error(err, "Failed to get cluster config")
		os.Exit(1)
	}

	// Log the API server URL for debugging
	zlog.Info("Kubernetes API server", "url", config.Host, "insecure", config.Insecure)

	// Parse the API server URL
	targetURL, err := url.Parse(config.Host)
	if err != nil {
		zlog.Error(err, "Failed to parse API server URL")
		os.Exit(1)
	}

	// Create a reverse proxy to the Kubernetes API server
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	// Stream-friendly settings and resilient error handling (eg: kubectl logs --follow)
	proxy.FlushInterval = 200 * time.Millisecond
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		// Ignore client-side disconnects and aborted handlers
		if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrAbortHandler) {
			return
		}
		// Some transports wrap the error; handle by message substring as well
		if strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "request canceled") {
			return
		}
		zlog.Error(err, "proxy error")
		// Best-effort error response if headers not already sent
		http.Error(rw, "upstream error", http.StatusBadGateway)
	}

	// Configure the proxy to handle responses properly
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Add CORS headers for web-based clients
		resp.Header.Set("Access-Control-Allow-Origin", "*")
		resp.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		resp.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		return nil
	}

	// Configure transport with proper TLS settings
	// Skip TLS verification for the proxy (configurable via environment variable)
	skipTLSVerify := os.Getenv("PROXY_SKIP_TLS_VERIFY") != "false" // Default to true
	zlog.Info("Proxy TLS configuration", "skip_verify", skipTLSVerify)

	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
		// Add timeout settings
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Set Gin mode based on environment
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Create router with custom middleware
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(GinLogger())

	// Add authentication middleware
	router.Use(createAuthMiddleware(config, proxyConfig, zlog))

	// Define the main proxy route - this handles all Kubernetes API requests
	// Use a more specific pattern to avoid conflicts with health/ready endpoints
	router.Any("/api/*path", func(c *gin.Context) {
		// Handle CORS for OPTIONS requests
		if c.Request.Method == "OPTIONS" {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Status(http.StatusOK)
			return
		}

		// Log the incoming request
		zlog.Info("Proxying request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent())

		// Forward the request to the Kubernetes API server
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// Handle other Kubernetes API paths
	router.Any("/apis/*path", func(c *gin.Context) {
		// Handle CORS for OPTIONS requests
		if c.Request.Method == "OPTIONS" {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Status(http.StatusOK)
			return
		}

		// Log the incoming request
		zlog.Info("Proxying request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent())

		// Forward the request to the Kubernetes API server
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// Handle version endpoints
	router.Any("/version", func(c *gin.Context) {
		// Log the incoming request
		zlog.Info("Proxying request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent())

		// Forward the request to the Kubernetes API server
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// Handle metrics endpoints
	router.Any("/metrics", func(c *gin.Context) {
		// Log the incoming request
		zlog.Info("Proxying request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent())

		// Forward the request to the Kubernetes API server
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// Get port from environment or use default
	port := os.Getenv("PROXY_PORT")
	if port == "" {
		port = "8085"
		zlog.Info("Using default proxy port", "port", port)
	} else {
		zlog.Info("Using custom proxy port", "port", port)
	}

	// Get binding IP - check environment variable first, then WireGuard interface
	bindIP := os.Getenv("PROXY_BIND_IP")
	if bindIP == "" {
		bindIP = getWireGuardInterfaceIP()
	}

	addr := ":" + port
	if bindIP != "" {
		addr = bindIP + ":" + port
		zlog.Info("Binding proxy to specific IP", "ip", bindIP, "port", port)
	} else {
		zlog.Info("Binding proxy to all interfaces", "port", port)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
		// Add timeouts
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start the HTTP server
	zlog.Info("Starting Kubernetes API proxy", "addr", srv.Addr, "target", config.Host, "port", port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zlog.Error(err, "failed to start proxy server")
			os.Exit(1)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	zlog.Info("Shutting down proxy server...")

	// Note: Netclient sidecar runs as a separate container and handles its own shutdown

	// Create a shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zlog.Error(err, "Server shutdown error")
	} else {
		zlog.Info("Proxy server shutdown complete")
	}
}

// createAuthMiddleware creates authentication middleware for the proxy
func createAuthMiddleware(config *rest.Config, proxyConfig ProxyConfig, zlog logr.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// All proxy routes require authentication (API routes are on separate server)

		// Handle different proxy modes
		switch proxyConfig.Mode {
		case NoAuthMode:
			// NoAuth mode: proxy requests without authentication
			zlog.V(1).Info("NoAuth mode: proxying request without authentication",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"client_ip", c.ClientIP())

		case AuthMode:
			// Auth mode: impersonate requests using WireGuard peer identity
			clientIP := c.ClientIP()

			// Look up user and groups from IP mapping
			impersonateUser := proxyConfig.ImpersonateUser
			impersonateGroups := proxyConfig.ImpersonateGroups

			if mappedUser, mappedGroups, exists := globalUserIPMap.GetUserMapping(clientIP); exists {
				impersonateUser = mappedUser
				impersonateGroups = mappedGroups
				zlog.V(1).Info("Auth mode: using mapped user/group",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"client_ip", clientIP,
					"mapped_user", impersonateUser,
					"mapped_groups", impersonateGroups)
			} else {
				zlog.V(1).Info("Auth mode: using default user/group (no mapping found)",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"client_ip", clientIP,
					"default_user", impersonateUser,
					"default_groups", impersonateGroups)
			}

			// Set impersonation headers for Kubernetes API server
			if impersonateUser != "" {
				c.Request.Header.Set("Impersonate-User", impersonateUser)
			}
			if len(impersonateGroups) > 0 {
				c.Request.Header.Set("Impersonate-Group", strings.Join(impersonateGroups, ","))
			}

			// Add additional impersonation headers for better compatibility
			c.Request.Header.Set("Impersonate-Extra-Original-User", clientIP)
			c.Request.Header.Set("Impersonate-Extra-Original-Group", "wireguard-peers")

		default:
			zlog.Error(fmt.Errorf("unknown proxy mode: %s", proxyConfig.Mode), "Invalid proxy configuration")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid proxy configuration",
			})
			c.Abort()
			return
		}

		// Set up authentication for the proxy itself to connect to K8s API server
		if config.BearerToken != "" {
			// Use Bearer token authentication
			c.Request.Header.Set("Authorization", "Bearer "+config.BearerToken)
			zlog.V(1).Info("Using Bearer token authentication for proxy")
		} else if config.CertFile != "" && config.KeyFile != "" {
			// Client certificate authentication is handled by the transport
			zlog.V(1).Info("Using client certificate authentication for proxy")
		} else if config.Username != "" && config.Password != "" {
			// Basic authentication
			c.Request.SetBasicAuth(config.Username, config.Password)
			zlog.V(1).Info("Using basic authentication for proxy")
		} else {
			// Check if Authorization header is already present
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				zlog.Error(fmt.Errorf("no authentication method available for proxy"), "Proxy authentication failed")
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Proxy authentication required",
				})
				c.Abort()
				return
			}
			zlog.V(1).Info("Using existing Authorization header for proxy")
		}

		// Add additional headers for better compatibility
		c.Request.Header.Set("User-Agent", "netmaker-k8s-proxy/1.0")

		// Log the request details
		zlog.V(1).Info("Request processed",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
			"proxy_mode", proxyConfig.Mode)

		c.Next()
	}
}

// checkNetclientContainer checks if netclient is running by looking for WireGuard interfaces
func checkNetclientContainer() map[string]interface{} {
	status := map[string]interface{}{
		"running": false,
		"type":    "wireguard_interface_check",
		"message": "Checking for WireGuard interfaces",
	}

	// Check if WireGuard interface exists (indicates netclient is running)
	if _, err := os.Stat("/sys/class/net/wg0"); err == nil {
		status["running"] = true
		status["interface"] = "wg0"
		status["message"] = "WireGuard interface wg0 detected"
		return status
	}

	// Check for other common WireGuard interface names
	interfaces := []string{"netmaker", "nm-*", "wg1", "wg2"}
	for _, iface := range interfaces {
		if _, err := os.Stat(fmt.Sprintf("/sys/class/net/%s", iface)); err == nil {
			status["running"] = true
			status["interface"] = iface
			status["message"] = fmt.Sprintf("WireGuard interface %s detected", iface)
			return status
		}
	}

	// If no interface found, check if we can list network interfaces
	if interfaces, err := os.ReadDir("/sys/class/net/"); err == nil {
		var wgInterfaces []string
		for _, iface := range interfaces {
			name := iface.Name()
			if name == "wg0" || name == "netmaker" || name[:2] == "wg" {
				wgInterfaces = append(wgInterfaces, name)
			}
		}
		if len(wgInterfaces) > 0 {
			status["running"] = true
			status["interfaces"] = wgInterfaces
			status["message"] = fmt.Sprintf("Found WireGuard interfaces: %v", wgInterfaces)
		}
	}

	return status
}

// getWireGuardInterfaceIP finds the IP address of the WireGuard interface with retry logic
func getWireGuardInterfaceIP() string {
	// Common WireGuard interface names
	interfaceName := "netmaker"
	if os.Getenv("IFACE_NAME") != "" {
		interfaceName = os.Getenv("IFACE_NAME")
	}

	zlog := zap.New(zap.UseDevMode(true))
	zlog.Info("Searching for WireGuard interfaces with retry logic", "interfaces", interfaceName)

	// Retry configuration (configurable via environment variables)
	maxRetries := getEnvInt("WIREGUARD_RETRY_MAX_ATTEMPTS", 20)                                  // Increased from 10 to 20
	baseDelay := time.Duration(getEnvInt("WIREGUARD_RETRY_BASE_DELAY_SECONDS", 3)) * time.Second // Increased from 2 to 3
	maxDelay := time.Duration(getEnvInt("WIREGUARD_RETRY_MAX_DELAY_SECONDS", 30)) * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		zlog.Info("Attempting to find WireGuard interface", "attempt", attempt, "maxRetries", maxRetries)

		// Use Go's net package to get all network interfaces
		netInterfaces, err := net.Interfaces()
		if err != nil {
			zlog.Error(err, "Failed to get network interfaces", "attempt", attempt)
			continue
		}

		// Look for our target interfaces
		for _, netIface := range netInterfaces {

			if netIface.Name == interfaceName {
				zlog.Info("Found WireGuard interface", "interface", netIface.Name, "attempt", attempt)

				// Get addresses for this interface
				addrs, err := netIface.Addrs()
				if err != nil {
					zlog.Error(err, "Failed to get addresses for interface", "interface", netIface.Name, "attempt", attempt)
					continue
				}

				// Look for IPv4 addresses
				for _, addr := range addrs {
					if ipNet, ok := addr.(*net.IPNet); ok {
						ip := ipNet.IP
						// Check if it's IPv4 and not loopback
						if ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
							ipStr := ip.String()
							zlog.Info("Found IP address", "interface", netIface.Name, "ip", ipStr, "attempt", attempt)
							return ipStr
						}
					}
				}

				zlog.Info("Interface found but no valid IPv4 address", "interface", netIface.Name, "attempt", attempt)
			}

		}

		// If this is not the last attempt, wait before retrying
		if attempt < maxRetries {
			// Calculate delay with exponential backoff
			delay := baseDelay * time.Duration(attempt)
			if delay > maxDelay {
				delay = maxDelay
			}

			zlog.Info("Waiting before retry", "delay", delay, "nextAttempt", attempt+1)
			time.Sleep(delay)
		}
	}

	zlog.Error(nil, "Failed to find WireGuard interface after all retries", "maxRetries", maxRetries)
	return ""
}

// getEnvInt gets an integer value from environment variable with default fallback
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
