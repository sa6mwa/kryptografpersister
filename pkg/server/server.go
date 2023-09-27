package server

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sa6mwa/anystore"
	"github.com/sa6mwa/kryptografpersister/internal/pkg/crand"
)

const (
	ContentTypeHeader string = "Content-Type"
	AcceptHeader      string = "Accept"
	ApplicationJSON   string = "application/json;charset=utf-8"
)

// Data is the type of each incoming json map[string][]byte
// ({"key":"ciphertext"}). Each Data is stored under a unique key in
// the anystore.
type Data struct {
	Key        string `json:"key"`
	Ciphertext []byte `json:"ciphertext"`
}

func init() {
	gob.Register(Data{})
}

func logErr(l *log.Logger, r *http.Request, err error) string {
	str := fmt.Sprint(r.Method, " ", r.RequestURI, " from ", r.RemoteAddr, ": ", err.Error())
	l.Print(str)
	return str
}

func logMsg(l *log.Logger, r *http.Request, msg string) string {
	str := fmt.Sprint(r.Method, " ", r.RequestURI, " from ", r.RemoteAddr, ": ", msg)
	l.Print(str)
	return str
}

// LoggingMiddleware is a logging http.Handler.
func LoggingMiddleware(l *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.Println(r.Method, r.RequestURI, "from", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// Start starts the kryptografpersister HTTP server, serving the API
// on proto at addr using AnyStoreDB persistence file dbFile with
// encryptionKey. Logging is done through l (if nil, log.Default()
// will be used). srv can be used to initialize timeouts, etc (but
// Handler and Addr will be overwritten). Start returns an error
// channel that will return nil or error when server is closed, a
// terminator channel that, when closed, will terminate the http
// server. The actual listen address from net.Listen is returned as a
// string pointer. Usage example:
//
//	returnCh, terminator, addr, err := server.Start("tcp", ":0", dbFile, "lhOAmgGdrFnfnsysiFMTwTZ227LxlFemjuRL72yPkRo", log.Default(), nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//	//defer close(terminator)
//	defer close(returnCh)
//	log.Println("Reduntant log message, listening to %s", *addr)
//	go func() {
//		time.Sleep(5 * time.Second)
//		close(terminator)
//	}()
//	err = <-returnCh
//	if err != nil {
//		log.Fatal(err)
//	}
func Start(proto, addr, dbFile, encryptionKey string, l *log.Logger, srv *http.Server) (chan error, chan struct{}, *string, error) {
	anyStore, err := anystore.NewAnyStore(&anystore.Options{
		EnablePersistence: true,
		PersistenceFile:   dbFile,
		EncryptionKey:     encryptionKey,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	if l == nil {
		l = log.Default()
	}

	l.Printf("Successfully opened persistence file %q", dbFile)

	length, err := anyStore.Len()
	if err != nil {
		anyStore.Close()
		return nil, nil, nil, err
	}
	plural := "s"
	if length == 1 {
		plural = ""
	}
	l.Printf("Persistence file %q contains %d key"+plural, dbFile, length)

	mux := http.NewServeMux()

	if srv == nil {
		srv = &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  1 * time.Minute,
			WriteTimeout: 1 * time.Minute,
			IdleTimeout:  2 * time.Minute,
		}
	} else {
		srv.Handler = mux
	}

	mux.Handle("/", LoggingMiddleware(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(AcceptHeader, ApplicationJSON)
		w.Header().Set(ContentTypeHeader, ApplicationJSON)

		switch r.Method {
		case http.MethodPut:
			if d, err := StoreJsonKV(anyStore, r.Body); err != nil {
				logErr(l, r, err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(ToJson(&Msg{Msg: fmt.Sprintf("Error: unable to store key-value pairs, all pairs in this transaction rolled back: %v", err)}))
				return
			} else {
				length := len(d)
				if length > 1 {
					logMsg(l, r, fmt.Sprintf("persisted %d key-value pairs", length))
				} else if length == 0 {
					logMsg(l, r, "no key-value pairs persisted")
				} else {
					logMsg(l, r, fmt.Sprintf("persisted %d key-value pair", length))
				}
			}
		case http.MethodGet:
			// Despite 200 OK, it Will return a {"SERVER_ERROR":"error
			// message"} json in case something fails in the AnyStore Run
			// transaction. The client API will pick this up.
			w.WriteHeader(http.StatusOK)
			if err := anyStore.Run(func(s anystore.AnyStore) error {
				keys, err := s.Keys()
				if err != nil {
					return err
				}
				for i := range keys {
					v, err := s.Load(keys[i])
					if err != nil {
						return err
					}
					data, ok := v.(Data)
					if !ok {
						return fmt.Errorf("expected Data type, but got %T", data)
					}
					if data.Key == "SERVER_ERROR" {
						data.Key = "server_error"
					}
					kv := make(map[string][]byte, 0)
					kv[data.Key] = data.Ciphertext
					j, err := json.Marshal(&kv)
					if err != nil {
						return err
					}
					j = append(j, []byte("\n")...)
					if _, err := w.Write(j); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				logErr(l, r, err)
				w.WriteHeader(http.StatusInternalServerError)
				msg := map[string][]byte{
					"SERVER_ERROR": []byte(err.Error()),
				}
				w.Write(ToJson(msg))
				return
			}
			return
		case http.MethodPost, http.MethodDelete:
			logErr(l, r, errors.New("method not implemented yet"))
			w.WriteHeader(http.StatusNotImplemented)
			w.Write(ToJson(&Msg{Msg: "Method not implemented yet."}))
			return
		default:
			logErr(l, r, errors.New("bad request"))
			w.WriteHeader(http.StatusBadRequest)
			w.Write(ToJson(&Msg{Msg: fmt.Sprintf("%d %s", http.StatusBadRequest, http.StatusText(http.StatusBadRequest))}))
			return
		}
		w.Write(ToJson(&Msg{Msg: "OK"}))
		return
	})))

	// Default proto is tcp4
	if proto == "" {
		proto = "tcp4"
	}
	ln, err := net.Listen(proto, addr)
	if err != nil {
		anyStore.Close()
		return nil, nil, nil, err
	}
	lnAddr := ln.Addr().String()
	srv.Addr = lnAddr
	returnCh := make(chan error)
	terminator := make(chan struct{})
	listenAndServeCh := make(chan error)
	go func() {
		var e error
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
		select {
		case sig := <-signalChannel:
			l.Printf("Caught signal %q, shutting down.", sig.String())
			e = fmt.Errorf("caught signal %q", sig.String())
		case e = <-listenAndServeCh:
		case <-terminator:
		}
		signal.Stop(signalChannel)
		close(signalChannel)
		if err := srv.Shutdown(context.Background()); err != nil {
			l.Print("HTTP server Shutdown: ", err.Error())
		}
		anyStore.Close()
		if e == nil {
			returnCh <- err
		} else {
			if err != nil {
				returnCh <- fmt.Errorf("%w: %w", e, err)
			} else {
				returnCh <- e
			}
		}
	}()

	l.Print("Serving ", proto, " http requests on ", srv.Addr)
	go func() {
		if err := ListenAndServe(ln, srv); err != http.ErrServerClosed {
			listenAndServeCh <- err
			l.Printf("HTTP server ListenAndServe: %v", err)
		} else {
			listenAndServeCh <- nil
		}
	}()
	return returnCh, terminator, &lnAddr, nil
}

// PlainStart is a simple wrapper for Start, blocking until the HTTP
// server is terminated or on error.
func PlainStart(protocol, address, dbFile, encryptionKey string) error {
	returnCh, terminator, _, err := Start(protocol, address, dbFile, encryptionKey, nil, nil)
	if err != nil {
		return err
	}
	defer close(terminator)
	defer close(returnCh)
	err = <-returnCh
	if err != nil {
		return err
	}
	return nil
}

type Msg struct {
	Msg string `json:"message"`
}

func ToJson(v any) []byte {
	j, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return j
}

// RandomStamp returns time.Now().UTC() as time.Format
// "20060102T150405.999999999_{19 character random int63}". If one tm
// is provided in the optional variadic argument, the first time.Time
// from the tm slice is used instead of time.Now().UTC(). Intended
// usage of this function is for creating keys for a KV
// map[string][]byte pair sent as a json stream.
func RandomStamp(tm ...time.Time) string {
	format := "20060102T150405.999999999"
	t := time.Now().UTC()
	if len(tm) > 0 {
		t = tm[0]
	}
	return t.Format(format) + fmt.Sprintf("_%.19d", crand.Int63())
}

// StoreJsonKV stores a {"key":"base64_ciphertext"} json object from
// stream into the a AnyStore atomically with a unique random key (as
// AnyStore key) and ensures key does not exist before storing, all
// done in a Run transaction. The incoming KV pair is stored as a
// server.Data object. In case there is any error in the stream, all
// already stored key-value pairs are deleted (rolled back) and the
// function returns an error (i.e operation is atomic). If StoreJsonKV
// does not return an error, all KV pairs in the stream were
// successfully persisted to the AnyStore. Returns a Data slice with
// all persisted objects or error.
func StoreJsonKV(a anystore.AnyStore, stream io.Reader) ([]Data, error) {
	transaction := make([]Data, 0)
	j := json.NewDecoder(stream)
	for {
		var kv map[string][]byte
		if err := j.Decode(&kv); err == nil {
			// happy path
			for key, value := range kv {
				// store each received KV pair into the db
				transaction = append(transaction, Data{
					Key:        key,
					Ciphertext: value,
				})
			}
		} else if err == io.EOF {
			// done
			break
		} else {
			// not so happy path
			return nil, err // return 400 Bad Request
		}
	}
	// Store using a locked AnyStore, and rollback any stored data in
	// case of error.
	if err := a.Run(func(s anystore.AnyStore) error {
		keysToRollBack := make([]string, 0)
		for i := range transaction {
			key := RandomStamp()
			for {
				if s.HasKey(key) {
					key = RandomStamp()
				} else {
					break
				}
			}
			if err := s.Store(key, transaction[i]); err != nil {
				for _, k := range keysToRollBack {
					s.Delete(k)
				}
				return err
			}
			keysToRollBack = append(keysToRollBack, key)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return transaction, nil
}

func ListenAndServe(customListener net.Listener, srv *http.Server) error {
	return srv.Serve(customListener)
}
