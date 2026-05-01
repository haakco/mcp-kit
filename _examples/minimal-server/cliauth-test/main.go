package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/haakco/mcp-kit/cliauth"
)

func main() {
	issuer := flag.String("issuer", "http://localhost:8080", "OAuth issuer URL")
	credPath := flag.String("cred-path", "", "credential file override")
	flag.Parse()

	login, err := cliauth.NewLogin(cliauth.LoginOptions{
		Issuer:   *issuer,
		CredPath: *credPath,
		OpenURL: func(authURL string) error {
			go followAuthorizeURL(authURL)
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	token, err := login.GetAccessToken(ctx)
	if err == nil {
		fmt.Println("using cached token")
		mustListTools(ctx, *issuer, token)
		return
	}
	if err != nil && !errors.Is(err, cliauth.ErrNotLoggedIn) {
		log.Printf("cached token unavailable: %v", err)
	}

	if err := login.RunLoopback(ctx, cliauth.LoopbackOptions{Port: 0, Scope: "mcp.read"}); err != nil {
		log.Fatal(err)
	}
	token, err = login.GetAccessToken(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("minted token")
	mustListTools(ctx, *issuer, token)
}

func followAuthorizeURL(authURL string) {
	resp, err := http.Get(authURL)
	if err != nil {
		log.Printf("open authorize URL: %v", err)
		return
	}
	if err := resp.Body.Close(); err != nil {
		log.Printf("close authorize response: %v", err)
	}
}

func mustListTools(ctx context.Context, issuer, token string) {
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(issuer, "/")+"/mcp", body)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("tools/list status = %s", resp.Status)
	}
	fmt.Println("tools/list ok")
}
