package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func main() {
	var manifestPath string
	flag.StringVar(&manifestPath, "manifest", "", "exact release manifest")
	flag.Parse()
	if manifestPath == "" {
		fmt.Fprintln(os.Stderr, "Agent Runner host verification: ISOLATION_UNAVAILABLE")
		os.Exit(2)
	}
	manifest, err := agentrunner.LoadReleaseManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Agent Runner host verification: ISOLATION_UNAVAILABLE")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evidence, probeErr := (agentrunner.SystemProbe{Manifest: manifest}).Probe(ctx)
	encoded, _ := json.Marshal(evidence)
	fmt.Println(string(encoded))
	if probeErr != nil || !evidence.Ready {
		fmt.Fprintln(os.Stderr, "Agent Runner host verification: ISOLATION_UNAVAILABLE")
		os.Exit(2)
	}
	fmt.Println("Agent Runner host verification: READY")
}
