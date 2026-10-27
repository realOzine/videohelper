package desktop

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

type Connection struct {
	Base  string `json:"base"`
	Token string `json:"token"`
}

// Desktop exposes only this process's connection and the system file picker.
type Desktop struct{ connection Connection }

func (d *Desktop) Bootstrap() Connection { return d.connection }

type File struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (*Desktop) PickFiles() ([]File, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "VideoHelper", Multiple: true})
	if err != nil {
		return nil, err
	}
	files := make([]File, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
		}
		files = append(files, File{Path: path, Name: filepath.Base(path), Size: info.Size()})
	}
	return files, nil
}

func Run(connection Connection) error {
	mygo.App.SetName("VideoHelper")
	mygo.Theme.SetSource(mygo.ThemeDark)
	mygo.Bind(&Desktop{connection: connection})
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "VideoHelper", URL: "/search.html", Width: 1280, Height: 840,
			MinWidth: 760, MinHeight: 540, BackgroundColor: "#0f0e15", StateKey: "main",
		})
		win.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			if !ownPage(e.URL, mygo.IsDev()) {
				e.PreventDefault()
			}
		})
	})
	mygo.App.OnWindowAllClosed(mygo.App.Quit)
	return mygo.App.Run()
}

func ownPage(raw string, dev bool) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && ((u.Scheme == "mygo" && u.Host == "localhost") || (u.Scheme == "http" && u.Host == "mygo.localhost") || (dev && u.Scheme == "http" && u.Host == "127.0.0.1:5173"))
}
