package main

import (
	"context"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	//  "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/ec2imds"
)

const AppName = "k8s-demo-app"

var (
	listenAddr   string
	healthy      bool
	hostname     string
	zone         string
	node         string
	cluster      string
	message      string
	instanceType string
	arch         string
)

func main() {
	flag.StringVar(&listenAddr, "listen-addr", lookupEnvOrString("K8S_DEMO_APP_LISTEN_ADDR", ":8080"), "server listen address")
	flag.Parse()

	logger := log.New(os.Stdout, AppName+":", log.LstdFlags)

	logger.Println("Starting", AppName)

	// Using the SDK's default configuration, loading additional config
	// and credentials values from the environment variables, shared
	// credentials, and shared configuration files
	cfg, err := config.LoadDefaultConfig(config.WithRegion("us-west-2"))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	ctx := context.Background()

	client := ec2imds.NewFromConfig(cfg)
	region, err := client.GetRegion(ctx, nil)
	if err != nil {
		log.Fatalf("expect no error, got %v", err)
	}
	logger.Println("The region is:", region.Region)

	resp, err := client.GetInstanceIdentityDocument(ctx, nil)
	if err != nil {
		log.Fatalf("expect no error, got %v", err)
	}
	if resp == nil {
		log.Fatalf("expect resp, got none")
	}

	zone = resp.AvailabilityZone
	node = resp.InstanceID
	instanceType = resp.InstanceType
	arch = resp.Architecture
	hostname = lookupEnvOrString("MY_POD_NAME", "Pod Name")
	cluster = lookupEnvOrString("MY_POD_IP", "Pod IP")
	message = lookupEnvOrString("K8S_DEMO_APP_MESSAGE", "Hello K8s World!")

	// HTTP Server
	router := http.NewServeMux()
	fs := http.FileServer(http.Dir("static"))

	router.Handle("/", index())

	router.Handle("/healthz", healthz())
	router.Handle("/static/", http.StripPrefix("/static/", fs))

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      logging(logger)(defaultHeaders(router)),
		ErrorLog:     logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		logger.Println("Shutting down")
		healthy = false

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		logger.Println("Waiting for graceful period of 10s")
		time.Sleep(10 * time.Second)

		if err := server.Shutdown(ctx); err != nil {
			logger.Fatalf("Failed to shudown: %v\n", err)
		}
		close(done)
	}()

	healthy = true
	logger.Println("Listening at ", listenAddr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Failed to listen on %s: %v\n", listenAddr, err)
	}

	<-done
	logger.Println("Stopped")
}

func lookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func defaultHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", AppName)
		h.ServeHTTP(w, r)
	})
}

func index() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("template.html"))

		tempData := map[string]interface{}{
			"Zone":         zone,
			"Hostname":     hostname,
			"Node":         node,
			"Cluster":      cluster,
			"Message":      message,
			"InstanceType": instanceType,
			"Arch":         arch,
			"Path":         r.URL.Path,
		}

		tmpl.Execute(w, tempData)
	})
}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func logging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				logger.Println(r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
			}()
			next.ServeHTTP(w, r)
		})
	}
}
