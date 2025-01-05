package proxy

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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
	var config *rest.Config
	var err error
	if conf.InClusterCfg() {
		// Get in-cluster Kubernetes configuration
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	}
	if err != nil {
		zlog.Error(err, "Failed to get cluster config")
		os.Exit(1)
	}

	// Create Kubernetes client
	// clientset, err := kubernetes.NewForConfig(config)
	// if err != nil {
	// 	logger.Error(err, "Failed to create Kubernetes client")
	// 	os.Exit(1)
	// }

	// Get the Kubernetes API server URL from the configuration
	apiServerURL := config.Host

	// Parse the API server URL
	targetURL, err := url.Parse(apiServerURL)
	if err != nil {
		zlog.Error(err, "Failed to parse API server URL")
		os.Exit(1)
	}

	// Create a reverse proxy to the Kubernetes API server
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ModifyResponse = func(resp *http.Response) error {
		// You can customize the response here if needed
		return nil
	}
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	//gin.SetMode(gin.ReleaseMode)
	gin.SetMode(gin.DebugMode)

	// Register gin router with default configuration
	// comes with default middleware and recovery handlers.
	router := gin.Default()

	// Attach custom logger to gin to print incoming requests to stdout.
	router.Use(GinLogger())
	// Handle incoming requests

	// Add a middleware to inject the Kubernetes Bearer Token for authentication
	router.Use(func(c *gin.Context) {
		c.Request.Header.Set("Authorization", "Bearer "+config.BearerToken)
		c.Next()
	})
	// Define a proxy route
	router.Any("/*path", func(c *gin.Context) {
		// Forward the request to the Kubernetes API server
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	srv := &http.Server{
		Addr:    ":8085",
		Handler: router,
	}
	// Start the HTTP server
	zlog.Info("Starting Kubernetes API proxy", "addr", srv.Addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zlog.Error(err, "failed to start proxy server")
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	zlog.Info("Shutting Down Server ...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server Shutdown:", err)
	}
}
