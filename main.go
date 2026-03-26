package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/thingsdb/go-timod"
)

//go:embed bin/frontend/*
var frontendFiles embed.FS

type spaHandler struct {
	staticFS http.FileSystem
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "."
	}
	f, err := h.staticFS.Open(path)
	if err != nil {
		r.URL.Path = "/"
	} else {
		err := f.Close()
		if err != nil {
			return
		}
	}

	// Hand the request off to the standard Go FileServer
	http.FileServer(h.staticFS).ServeHTTP(w, r)
}

func handler(buf *timod.Buffer, quit chan bool) {
	for {
		select {
		case pkg := <-buf.PkgCh:
			switch pkg.Tp {
			case timod.ProtoModuleConf:
				timod.WriteConfOk()

			case timod.ProtoModuleReq:
				timod.WriteEx(pkg.Pid, timod.ExCancelled, "This module only serves a frontend")

			default:
				log.Printf("Error: Unexpected package type: %d", pkg.Tp)
			}

		case err := <-buf.ErrCh:
			log.Printf("Error: %s", err)
			quit <- true
		}
	}
}

func startUIServer() {
	distFolder, err := fs.Sub(frontendFiles, "bin/frontend")
	if err != nil {
		log.Fatal("Failed to load embedded dist folder: ", err)
	}

	http.Handle("/", spaHandler{staticFS: http.FS(distFolder)})

	fmt.Println("Front-end UI running at http://localhost:8181")
	if err := http.ListenAndServe(":8181", nil); err != nil {
		log.Fatal("Web server failed: ", err)
	}
}

func main() {
	go startUIServer()

	timod.StartModule("flow_engine", handler)
}
