package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/workers"
)

func main() {
	backend := flag.String("backend", "", "backend name")
	workspace := flag.String("workspace", ".", "workspace root")
	flag.Parse()
	implementation, err := workers.ImplementationFor(ael.Backend(*backend))
	if err != nil {
		log.Fatal(err)
	}
	server, err := workers.NewServer(*workspace, implementation)
	if err != nil {
		log.Fatal(err)
	}
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
