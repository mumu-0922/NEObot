package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "MCP manifest validation failed")
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp-validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "path to the MCP manifest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: mcp-validate --manifest FILE")
	}
	if strings.TrimSpace(*manifestPath) == "" || strings.TrimSpace(*manifestPath) != *manifestPath {
		return errors.New("manifest path is required")
	}
	catalog, err := mcpclient.LoadCatalog()
	if err != nil {
		return err
	}
	servers, err := mcpclient.LoadManifest(*manifestPath, os.LookupEnv)
	if err != nil {
		return err
	}
	remote, stdio := 0, 0
	for _, server := range servers {
		switch server.Transport {
		case mcpclient.TransportStreamableHTTP:
			remote++
		case mcpclient.TransportStdio:
			stdio++
		default:
			return errors.New("manifest contains an unsupported transport")
		}
	}
	_, err = fmt.Fprintf(
		output,
		"MCP manifest valid: catalog=%d manifest=%d remote=%d stdio=%d\n",
		len(catalog.Servers), len(servers), remote, stdio,
	)
	return err
}
