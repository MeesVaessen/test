package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	thingsdb "github.com/thingsdb/go-thingsdb"
	timod "github.com/thingsdb/go-timod"
	"github.com/vmihailenco/msgpack"
)

// 1. Point embed to your new directory. No leading slash!
//
//go:embed bin/frontend/*
var frontendFiles embed.FS

type request struct {
	Scope string      `msgpack:"scope"`
	Name  *string     `msgpack:"name"`
	Args  interface{} `msgpack:"args"`
}

var conn *thingsdb.Conn = nil

func handler(buf *timod.Buffer, quit chan bool) {
	for {
		select {
		case pkg := <-buf.PkgCh:
			switch timod.Proto(pkg.Tp) {
			case timod.ProtoModuleConf:
				// Acknowledge the configuration request
				timod.WriteConfOk()

			case timod.ProtoModuleReq:
				// Unpack the incoming request
				var req request
				if err := msgpack.Unmarshal(pkg.Data, &req); err != nil {
					timod.WriteEx(pkg.Pid, timod.ExBadData, "Invalid request")
					continue
				}

				// Check if ThingsDB is calling the "import_schema" method we exposed
				if req.Name != nil && *req.Name == "import_schema" {

					// 1. Read your schema.ti file using the embed.FS package
					// (Assuming you embedded it alongside your frontend files)
					schemaBytes, err := frontendFiles.ReadFile("schema.ti")
					if err != nil {
						timod.WriteEx(pkg.Pid, timod.ExOperation, "Failed to read schema.ti from binary")
						continue
					}
					schemaCode := string(schemaBytes)

					// 2. Ensure we have an active client connection to ThingsDB
					if conn == nil {
						timod.WriteEx(pkg.Pid, timod.ExCancelled, "Module client connection not initialized")
						continue
					}

					// 3. Execute the schema code against the collection that called the module
					// req.Scope automatically contains the scope (e.g., "//my_collection")
					_, err = conn.Query(req.Scope, schemaCode, nil)
					if err != nil {
						timod.WriteEx(pkg.Pid, timod.ExOperation, fmt.Sprintf("Schema import failed: %v", err))
						continue
					}

					// 4. Send a successful response back to the user
					resData, _ := msgpack.Marshal("Schema imported successfully!")
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
	// 2. Remove the leading slash here!
	// Use "bin/frontend" INSTEAD OF "/bin/frontend"
	distFolder, err := fs.Sub(frontendFiles, "bin/frontend")
	if err != nil {
		log.Fatal("Failed to load embedded dist folder: ", err)
	}

	http.Handle("/", http.FileServer(http.FS(distFolder)))

	fmt.Println("Front-end UI running at http://localhost:8181")
	if err := http.ListenAndServe(":8181", nil); err != nil {
		log.Fatal("Web server failed: ", err)
	}
}

func main() {
	go startUIServer()

	conn = thingsdb.NewConn("localhost", 9200, nil)
	if err := conn.Connect(); err != nil {
		log.Printf("Failed to connect to ThingsDB: %s", err)
		conn = nil
	} else {
		if err := conn.AuthPassword("admin", "pass"); err != nil {
			log.Printf("Failed to authenticate to ThingsDB: %s", err)
			conn = nil
		} else {
			log.Println("Module client successfully connected to ThingsDB!")
		}
	}

	// 3. Start de ThingsDB module protocol handler.
	// Dit vervangt de 'select {}', blokkeert de main thread, en handelt de communicatie af.
	// Het eerste argument ("flow_engine") moet overeenkomen met de naam van je module.
	timod.StartModule("flow_engine", handler)

	if conn != nil {
		conn.Close()
	}
}
