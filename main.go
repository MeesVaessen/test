package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	timod "github.com/thingsdb/go-timod"
	"github.com/vmihailenco/msgpack"
)

//go:embed bin/frontend/* schema.ti
var embeddedFiles embed.FS

type request struct {
	Scope string      `msgpack:"scope"`
	Name  *string     `msgpack:"name"`
	Args  interface{} `msgpack:"args"`
}

func handler(buf *timod.Buffer, quit chan bool) {
	for {
		select {
		case pkg := <-buf.PkgCh:
			switch timod.Proto(pkg.Tp) {
			case timod.ProtoModuleConf:
				timod.WriteConfOk()

			case timod.ProtoModuleReq:
				var req request
				if err := msgpack.Unmarshal(pkg.Data, &req); err != nil {
					timod.WriteEx(pkg.Pid, timod.ExBadData, "Invalid request")
					continue
				}

				if req.Name != nil && *req.Name == "import_schema" {

					schemaBytes, err := embeddedFiles.ReadFile("schema.ti")
					if err != nil {
						timod.WriteEx(pkg.Pid, timod.ExOperation, "Failed to read schema.ti")
						continue
					}

					resData, _ := msgpack.Marshal(schemaBytes)
					timod.WriteResponseRaw(pkg.Pid, resData)

				} else {
					// Handle unknown procedures
					timod.WriteEx(pkg.Pid, timod.ExBadData, "Unknown procedure name")
				}

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
	distFolder, err := fs.Sub(embeddedFiles, "bin/frontend")
	if err != nil {
		log.Fatal("Failed to load embedded dist folder: ", err)
	}

	http.Handle("/", http.FileServer(http.FS(distFolder)))

	fmt.Println("Front-end UI running at http://localhost:8182")
	if err := http.ListenAndServe(":8182", nil); err != nil {
		log.Fatal("Web server failed: ", err)
	}
}

func main() {
	go startUIServer()

	timod.StartModule("test", handler)
}
