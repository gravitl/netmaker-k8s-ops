package proxy

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// StartAPIServer starts the API server for admin/management endpoints
func StartAPIServer(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	zlog := zap.New(zap.UseDevMode(true))

	// Set Gin mode based on environment
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Create API router with custom middleware
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(GinLogger())

	// Add health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"api":    "netmaker-k8s-api",
		})
	})

	// Add readiness check endpoint
	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"api":    "netmaker-k8s-api",
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

	// Add user IP mapping management endpoints
	router.GET("/admin/user-mappings", func(c *gin.Context) {
		mappings := GetAllUserIPMappings()
		c.JSON(http.StatusOK, gin.H{
			"status": "user_mappings",
			"data":   mappings,
		})
	})

	router.POST("/admin/user-mappings", func(c *gin.Context) {
		var request struct {
			IP     string   `json:"ip" binding:"required"`
			User   string   `json:"user" binding:"required"`
			Groups []string `json:"groups"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"details": err.Error(),
			})
			return
		}

		SetUserIPMapping(request.IP, request.User, request.Groups)
		zlog.Info("User IP mapping added", "ip", request.IP, "user", request.User, "groups", request.Groups)

		c.JSON(http.StatusOK, gin.H{
			"status": "mapping_added",
			"ip":     request.IP,
			"user":   request.User,
			"groups": request.Groups,
		})
	})

	router.DELETE("/admin/user-mappings/:ip", func(c *gin.Context) {
		ip := c.Param("ip")
		if ip == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "IP parameter is required",
			})
			return
		}

		RemoveUserIPMapping(ip)
		zlog.Info("User IP mapping removed", "ip", ip)

		c.JSON(http.StatusOK, gin.H{
			"status": "mapping_removed",
			"ip":     ip,
		})
	})

	// Add external API sync endpoint
	router.POST("/admin/sync-external-api", func(c *gin.Context) {
		externalAPIConfig := getNMAPIConfig()
		if externalAPIConfig.ServerDomain == "" || externalAPIConfig.APIToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "External API not configured",
			})
			return
		}

		if err := fetchUserMappingsFromAPI(externalAPIConfig, zlog); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to sync from external API",
				"details": err.Error(),
			})
			return
		}

		zlog.Info("Manual external API sync completed")
		c.JSON(http.StatusOK, gin.H{
			"status": "sync_completed",
			"server": externalAPIConfig.ServerDomain,
		})
	})

	// Get port from environment or use default
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8086"
		zlog.Info("Using default API port", "port", port)
	} else {
		zlog.Info("Using custom API port", "port", port)
	}

	// Get binding IP - check environment variable first, then WireGuard interface
	bindIP := os.Getenv("API_BIND_IP")
	if bindIP == "" {
		bindIP = getWireGuardInterfaceIP()
	}

	addr := ":" + port
	if bindIP != "" {
		addr = bindIP + ":" + port
		zlog.Info("Binding API server to specific IP", "ip", bindIP, "port", port)
	} else {
		zlog.Info("Binding API server to all interfaces", "port", port)
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
	zlog.Info("Starting API server", "addr", srv.Addr, "port", port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zlog.Error(err, "failed to start API server")
			os.Exit(1)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	zlog.Info("Shutting down API server...")

	// Create a shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zlog.Error(err, "API server shutdown error")
	} else {
		zlog.Info("API server shutdown complete")
	}
}

