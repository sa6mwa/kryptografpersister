// kryptografpersister is an API server persisting json kryptograf
// messages (map[string][]byte) or any other json key-value pair in
// the format {"key_string":"base64_encoded_byte_slice"}.
//
// Install:
//
//	GOBIN=$HOME/bin go install github.com/sa6mwa/kryptografpersister@latest
//
// Usage:
//
//	$ bin/kryptografpersister -h
//	Usage of bin/kryptografpersister:
//		-addr string
//			Address to bind the Persister http server to (default ":11185")
//		-db string
//			AnyStore DB file used as backend for the storage API (default "kryptografpersister.db")
//		-encryption-key-env string
//			Environment variable to retrieve the encryption key used to
//			load and store data in the AnyStoreDB (default
//			"PERSISTER_ENCRYPTION_KEY")
//		-protocol string
//			Network protocol to listen on (default "tcp4")
//
//	$ bin/kryptografpersister -db
//	$ bin/kryptografpersister -db ~/test.db
//	2023/09/27 21:46:14 server.go:99: Successfully opened persistence file "/home/sa6mwa/test.db"
//	2023/09/27 21:46:14 server.go:110: Persistence file "/home/sa6mwa/test.db" contains 0 keys
//	2023/09/27 21:46:14 server.go:248: Serving tcp4 http requests on 0.0.0.0:11185
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sa6mwa/kryptografpersister/pkg/server"
)

const (
	DefaultAddress          string = ":11185"
	DefaultProtocol         string = "tcp4"
	DefaultEncryptionKey    string = "Z6pT9Iw+YTiRtyIuNjn3q0vwc6BSZpPFpZn7sH606xU"
	DefaultEncryptionKeyEnv string = "PERSISTER_ENCRYPTION_KEY"
	DefaultAnyStoreDBFile   string = "kryptografpersister.db"
)

var (
	listenTo         string
	protocol         string
	encryptionKey    string
	encryptionKeyEnv string
	dbFile           string
)

func init() {
	flag.StringVar(&protocol, "protocol", DefaultProtocol, "Network protocol to listen on")
	flag.StringVar(&listenTo, "addr", DefaultAddress, "Address to bind the Persister http server to")
	flag.StringVar(&encryptionKeyEnv, "encryption-key-env", DefaultEncryptionKeyEnv, "Environment variable to retrieve the encryption key used to load and store data in the AnyStoreDB")
	flag.StringVar(&dbFile, "db", DefaultAnyStoreDBFile, "AnyStore DB file used as backend for the storage API")
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lshortfile)
	flag.Parse()
	if encryptionKey = strings.TrimSpace(os.Getenv(encryptionKeyEnv)); encryptionKey == "" {
		encryptionKey = DefaultEncryptionKey
	}
	returnCh, terminator, _, err := server.Start(protocol, listenTo, dbFile, encryptionKey, log.Default(), &http.Server{
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  5 * time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer close(terminator)
	defer close(returnCh)

	err = <-returnCh
	if err != nil {
		log.Fatal(err)
	} else {
		log.Println("OK")
	}
}
