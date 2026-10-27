package main

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	assets "github.com/ozine/videohelper"
	"github.com/ozine/videohelper/desktop"
	"github.com/ozine/videohelper/internal/desktop/processor"
	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
	"github.com/ozine/videohelper/internal/desktop/server"
	"github.com/ozine/videohelper/internal/desktop/store"
)

func main() {
	if os.Getenv("MYGO_GENERATE") != "" {
		if err := desktop.Run(desktop.Connection{}); err != nil {
			log.Print(err)
		}
		return
	}
	dataDir, err := applicationDataDir()
	if err != nil {
		log.Fatal(err)
	}
	launched, err := desktop.LaunchBundle(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	if launched {
		return
	}
	mygo.App.SetName("VideoHelper")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.OnSecondInstance(func([]string, string) {
		for _, win := range mygo.Windows() {
			win.Restore()
			win.Show()
			win.Focus()
		}
	})
	if err := run(dataDir); err != nil && !errors.Is(err, context.Canceled) {
		log.Print(err)
		if !mygo.App.IsReady() {
			mygo.App.WhenReady(func() { mygo.Dialog.Error("VideoHelper", err.Error()); mygo.App.Quit() })
			_ = mygo.App.Run()
		}
	}
}

func applicationDataDir() (string, error) {
	dataDir := os.Getenv("VIDEOHELPER_DATA_DIR")
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(base, "VideoHelper")
	}
	return filepath.Abs(dataDir)
}

func run(dataDir string) error {
	mygo.App.SetPath(mygo.PathUserData, dataDir)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, dataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	logFile, err := os.OpenFile(filepath.Join(dataDir, "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, logFile), nil)))
	manager := desktopruntime.New(dataDir, assets.RuntimeAssets)
	var mu sync.RWMutex
	var engine *processor.Processor
	var startupErr error
	var waiting bool
	retry := make(chan struct{}, 1)
	api, err := server.Start(server.Deps{
		Store: db, Dev: mygo.IsDev(),
		Processor: func() *processor.Processor { mu.RLock(); defer mu.RUnlock(); return engine },
		State: func() any {
			mu.RLock()
			defer mu.RUnlock()
			state := manager.State()
			if startupErr != nil {
				state.Ready = false
				state.Stage = "failed"
				state.Error = startupErr.Error()
			}
			return state
		},
		Retry: func() error {
			mu.Lock()
			defer mu.Unlock()
			if !waiting {
				return store.ErrConflict
			}
			waiting = false
			retry <- struct{}{}
			return nil
		},
	})
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			err := manager.Ensure(ctx, nil)
			if err == nil {
				var services desktopruntime.Services
				services, err = manager.Start(ctx)
				if err == nil {
					var p *processor.Processor
					p, err = processor.New(ctx, db, processor.Config{Tools: manager.Tools(), Endpoints: services.Endpoints, Token: services.Token, VisualSpace: services.VisualSpace, AudioSpace: services.AudioSpace, Transcriber: services.Transcriber})
					if err == nil {
						mu.Lock()
						engine = p
						mu.Unlock()
						err = superviseProcessor(ctx, p.Run, manager.State)
					}
				}
			}
			if err == nil && ctx.Err() == nil {
				err = errors.New("Local processing stopped unexpectedly")
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				_ = manager.Close(stopCtx)
				stop()
				mu.Lock()
				startupErr = err
				engine = nil
				waiting = true
				mu.Unlock()
				log.Print(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-retry:
				mu.Lock()
				manager = desktopruntime.New(dataDir, assets.RuntimeAssets)
				startupErr = nil
				mu.Unlock()
			}
		}
	}()
	uiStopped := make(chan struct{})
	go quitWhenCanceled(ctx, uiStopped, mygo.App.Quit)
	err = desktop.Run(desktop.Connection{Base: api.Base, Token: api.Token})
	close(uiStopped)
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	closeErr := api.Close(shutdown)
	mu.RLock()
	currentManager := manager
	mu.RUnlock()
	runtimeErr := currentManager.Close(shutdown)
	select {
	case <-done:
	case <-shutdown.Done():
		return errors.Join(err, closeErr, runtimeErr, shutdown.Err())
	}
	return errors.Join(err, closeErr, runtimeErr)
}
