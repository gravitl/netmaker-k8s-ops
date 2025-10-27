package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/gravitl/netmaker-k8s-ops/conf"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

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
	router.Use(createAuthMiddleware(config, zlog))

	// Add health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"proxy":  "k8s-api-proxy",
		})
	})

	// Add readiness check endpoint
	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"proxy":  "k8s-api-proxy",
		})
	})

	// Add netclient status endpoint (simplified - just checks WireGuard interface)
	router.GET("/netclient/status", func(c *gin.Context) {
		netclientStatus := checkNetclientContainer()
		c.JSON(http.StatusOK, gin.H{
			"status": "netclient_status",
			"data":   netclientStatus,
		})
	})

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
func createAuthMiddleware(config *rest.Config, zlog logr.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip authentication for health checks
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
			c.Next()
			return
		}

		// Handle different authentication methods
		if config.BearerToken != "" {
			// Use Bearer token authentication
			c.Request.Header.Set("Authorization", "Bearer "+config.BearerToken)
			zlog.V(1).Info("Using Bearer token authentication")
		} else if config.CertFile != "" && config.KeyFile != "" {
			// Client certificate authentication is handled by the transport
			zlog.V(1).Info("Using client certificate authentication")
		} else if config.Username != "" && config.Password != "" {
			// Basic authentication
			c.Request.SetBasicAuth(config.Username, config.Password)
			zlog.V(1).Info("Using basic authentication")
		} else {
			// Check if Authorization header is already present
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				zlog.Error(fmt.Errorf("no authentication method available"), "Authentication failed")
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Authentication required",
				})
				c.Abort()
				return
			}
			zlog.V(1).Info("Using existing Authorization header")
		}

		// Add additional headers for better compatibility
		c.Request.Header.Set("User-Agent", "netmaker-k8s-proxy/1.0")

		// Log the authentication method being used
		zlog.V(1).Info("Request authenticated",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP())

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
