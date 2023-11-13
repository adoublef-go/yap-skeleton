package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rs/xid"
)

var addr = flag.String("addr", ":8080", "bind listen addr")
var cluster = flag.String("cluster", "nats-route://0.0.0.0:4248", "bind cluster routes")
var storeDir = "./data/example"

var (
	timeout = 5 * time.Second
	buffer  = 128 * 1024 * 1024
)

func main() {
	flag.Parse()

	err := run(context.Background())
	if err != nil {
		log.Fatalln(err)
	}
}

func run(ctx context.Context) (err error) {
	if *addr == "" {
		return errors.New("address not set")
	}

	if *cluster == "" {
		return errors.New("routes not set")
	}

	ns, err := server.NewServer(&server.Options{
		JetStream:  true,
		StoreDir:   storeDir,
		ServerName: xid.New().String(),
		Port:       4222,
		HTTPPort:   8222,
		Cluster: server.ClusterOpts{
			Name: "NATS",
			Port: 4248,
			// Username: "",
			// Password: "",
		},
		Routes:    server.RoutesFromStr(*cluster),
		RoutesStr: *cluster,
	})
	if err != nil {
		return err
	}
	ns.Start()
	opt := nats.Options{
		InProcessServer:  ns,
		AllowReconnect:   true,
		MaxReconnect:     -1,
		ReconnectWait:    timeout,
		Timeout:          timeout,
		ReconnectBufSize: buffer,
	}
	nc, err := opt.Connect()
	if err != nil {
		return err
	}
	defer nc.Close()

	// create jetstream
	jsc, err := nc.JetStream()
	if err != nil {
		return err
	}
	_ = jsc

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex())
	mux.HandleFunc("/sse", handleEvent(nc))
	mux.HandleFunc("/submit", handleSubmit(nc))

	return http.ListenAndServe(*addr, mux)
}

//go:embed index.html
var index embed.FS

func handleIndex() http.HandlerFunc {
	var funcs = template.FuncMap{
		"env": func(key string) string {
			return os.Getenv(key)
		},
	}

	t := template.Must(template.New("").Funcs(funcs).ParseFS(index, "index.html"))
	return func(w http.ResponseWriter, r *http.Request) {
		t.ExecuteTemplate(w, "index.html", nil)
	}
}

func handleEvent(nc *nats.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set the content type to text/event-stream.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Continuously write messages to the client.
		sub, err := nc.SubscribeSync("submit")
		if err != nil {
			http.Error(w, "Failed to connect", http.StatusInternalServerError)
			return
		}

		defer sub.Unsubscribe()
		for {
			m, err := sub.NextMsgWithContext(r.Context())
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(m.Data))
			w.(http.Flusher).Flush()
		}
	}
}

func handleSubmit(nc *nats.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			message := r.FormValue("message")
			nc.Publish("submit", []byte(message))
		}
		// have it refresh for the current user, and not for other users
		w.WriteHeader(200)
	}
}
