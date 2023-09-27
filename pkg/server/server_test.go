package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/sa6mwa/kryptograf"
)

var (
	proto         string = "tcp4"
	addr          string = "localhost:0"
	encryptionKey string = "lhOAmgGdrFnfnsysiFMTwTZ227LxlFemjuRL72yPkRo"
)

func TestStart1(t *testing.T) {
	f, err := os.CreateTemp("", "persistence-*.db")
	dbFile := f.Name()
	f.Close()
	defer os.Remove(dbFile)
	returnCh, terminator, listenAddr, err := Start(proto, addr, dbFile, encryptionKey, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(terminator)
		err := <-returnCh
		if err != nil {
			t.Fatal(err)
		}
	}()
	t.Log("Listening on", *listenAddr)
	kurl := "http://" + *listenAddr

	resp, err := http.Get(kurl)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		close(terminator)
		t.Fatal(err)
	}
	if got, expected := resp.StatusCode, http.StatusOK; got != expected {
		t.Errorf("Expected status code %d, got %d", expected, got)
	}
	if got, expected := len(body), 0; got != expected {
		t.Errorf("Expected body length %d, got %d", expected, got)
	}
}

func TestStart2(t *testing.T) {
	f, err := os.CreateTemp("", "persistence-*.db")
	dbFile := f.Name()
	f.Close()
	defer os.Remove(dbFile)
	returnCh, terminator, listenAddr, err := Start(proto, addr, dbFile, encryptionKey, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(terminator)
		err := <-returnCh
		if err != nil {
			t.Fatal(err)
		}
	}()
	t.Log("Listening on", *listenAddr)
	kurl := "http://" + *listenAddr

	k, err := kryptograf.NewKryptograf().SetEncryptionKey(kryptograf.DefaultEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}

	p, err := kryptograf.NewPersistenceClient(kurl, "", k)
	if err != nil {
		t.Fatal(err)
	}

	if err := p.Store(context.Background(), "test", []byte("Hello world")); err != nil {
		t.Fatal(err)
	}

	if err := p.LoadAll(context.Background(), func(key string, plaintext []byte, err error) error {
		if err == io.EOF {
			return kryptograf.ErrStop
		}
		t.Log("key:", key, "plaintext:", string(plaintext))
		if got, expected := key, "test"; got != expected {
			t.Errorf("Expected key %q, but got %q", expected, got)
		}
		if got, expected := string(plaintext), "Hello world"; got != expected {
			t.Errorf("Expected plaintext %q, but got %q", expected, got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	ts := []map[string][]byte{
		{"key1": []byte("Hello world one")},
		{"key2": []byte("Hello world two")},
		{"key3": []byte("Hello world three")},
	}
	count := 0
	nextKV := func() (string, []byte, error) {
		if count >= len(ts) {
			return "", nil, kryptograf.ErrStop
		}
		for k, v := range ts[count] {
			count++
			return k, v, nil
		}
		return "", nil, errors.New("no key-value pairs")
	}
	if err := p.StoreFunc(context.Background(), func() (string, []byte, error) {
		return nextKV()
	}); err != nil {
		t.Fatal(err)
	}

	gots := []bool{false, false, false, false}
	if err := p.LoadAll(context.Background(), func(key string, plaintext []byte, err error) error {
		if err == io.EOF {
			return kryptograf.ErrStop
		}
		t.Log("key:", key, "plaintext:", string(plaintext))
		switch key {
		case "test":
			if got, expected := string(plaintext), "Hello world"; got != expected {
				t.Errorf("Expected %q, but got %q for %q", expected, got, key)
			}
			gots[0] = true
		case "key1":
			if got, expected := string(plaintext), "Hello world one"; got != expected {
				t.Errorf("Expected %q, but got %q for %q", expected, got, key)
			}
			gots[1] = true
		case "key2":
			if got, expected := string(plaintext), "Hello world two"; got != expected {
				t.Errorf("Expected %q, but got %q for %q", expected, got, key)
			}
			gots[2] = true
		case "key3":
			if got, expected := string(plaintext), "Hello world three"; got != expected {
				t.Errorf("Expected %q, but got %q for %q", expected, got, key)
			}
			gots[3] = true
		default:
			t.Errorf("Did not expect key %q with plaintext %q", key, string(plaintext))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for i, got := range gots {
		if !got {
			t.Errorf("Expected to have gotten result index %d, but it's marked %v", i, got)
		}
	}

}
