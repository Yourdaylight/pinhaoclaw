package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pinhaoclaw/pinhaoclaw/claw"
	"github.com/pinhaoclaw/pinhaoclaw/sharing"
)

func main() {
	action := flag.String("action", "package-info", "Action to run: package-info, deploy-node, bind-weixin")
	nodeID := flag.String("node", "", "Node ID for deploy-node")
	lobsterID := flag.String("lobster", "", "Lobster ID for bind-weixin")
	dataDir := flag.String("data-dir", defaultDataDir(), "Path to the pinhaoclaw data directory")
	timeout := flag.Duration("timeout", 5*time.Minute, "Operation timeout")
	flag.Parse()

	store := sharing.NewStore(*dataDir)
	svc := claw.NewNodeService(store)

	switch *action {
	case "package-info":
		info := svc.GetPicoclawPackageInfo()
		fmt.Printf("configured_path=%s\n", info.ConfiguredPath)
		fmt.Printf("resolved_path=%s\n", info.ResolvedPath)
		fmt.Printf("version=%s\n", info.Version)
		fmt.Printf("managed_path=%s\n", info.ManagedPath)
		fmt.Printf("supports_weixin_auth=%t\n", info.SupportsWeixin)
	case "deploy-node":
		mustNotEmpty(*nodeID, "-node is required for deploy-node")
		node := store.GetNode(*nodeID)
		if node == nil {
			fatalf("node %s not found", *nodeID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		events := make(chan claw.SSEEvent, 32)
		go svc.Deploy(ctx, node, events)
		for event := range events {
			fmt.Printf("event=%s data=%v\n", event.Event, event.Data)
		}
	case "bind-weixin":
		mustNotEmpty(*lobsterID, "-lobster is required for bind-weixin")
		lobster := store.GetLobster(*lobsterID)
		if lobster == nil {
			fatalf("lobster %s not found", *lobsterID)
		}
		node := store.GetNode(lobster.NodeID)
		if node == nil {
			fatalf("node %s not found", lobster.NodeID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		outCh, errCh := svc.BindWeixin(ctx, node, lobster.ID)
		for outCh != nil || errCh != nil {
			select {
			case line, ok := <-outCh:
				if !ok {
					outCh = nil
					continue
				}
				fmt.Printf("stdout=%s\n", line)
			case err, ok := <-errCh:
				if !ok {
					errCh = nil
					continue
				}
				fmt.Printf("stderr=%v\n", err)
			}
		}
	default:
		fatalf("unknown action %q", *action)
	}
}

func defaultDataDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "./~/.pinhaoclaw"
	}
	return filepath.Join(wd, "~", ".pinhaoclaw")
}

func mustNotEmpty(value, message string) {
	if value == "" {
		fatalf(message)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
