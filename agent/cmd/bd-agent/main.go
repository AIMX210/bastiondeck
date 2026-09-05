// Command bd-agent is the optional reverse-connecting agent. It enrolls with a
// BastionDeck server using a one-time secret, then executes approved commands
// and serves bounded filesystem operations over a single WebSocket.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"bd-agent/internal/client"
)

var version = "0.1.0"

func main() {
	var (
		server = flag.String("server", envOr("BD_AGENT_SERVER", "http://127.0.0.1:8840"), "server URL")
		secret = flag.String("secret", envOr("BD_AGENT_SECRET", ""), "enrollment secret")
		name   = flag.String("name", envOr("BD_AGENT_NAME", ""), "friendly name (informational)")
	)
	flag.Parse()
	if *secret == "" {
		log.Fatal("enrollment secret required (--secret or BD_AGENT_SECRET)")
	}
	log.SetFlags(log.LstdFlags)
	log.Printf("bd-agent %s starting, server=%s name=%q", version, *server, *name)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ag := client.New(client.Config{
		ServerURL: *server, Secret: *secret, Version: version, Name: *name})
	if err := ag.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
	log.Println("agent stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
