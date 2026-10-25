package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/ozine/videohelper/internal/config"
)

func TestAPIServesHealthRequiresAuthAndShutsDown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	cfg := config.Config{Server: config.ServerConfig{Addr: address},
		Auth: config.AuthConfig{Issuer: "http://127.0.0.1:1", Audience: "videohelper-test", JWKSURL: "http://127.0.0.1:1/jwks"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runAPI(ctx, nil, &runtime{}, cfg) }()
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get("http://" + address + "/healthz")
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("health=%d", response.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	response, err := client.Get("http://" + address + "/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated API status=%d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("API did not shut down")
	}
}
