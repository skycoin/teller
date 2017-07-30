package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"sync"

	"flag"

	"path/filepath"

	"encoding/json"

	"bytes"
	"io/ioutil"

	"github.com/google/gops/agent"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/proxy"
)

var (
	appDir         = ".teller-proxy"
	privateKeyFile = "key.json"
)

type privateKey struct {
	PrivateKey string `json:"private_key"`
}

func main() {
	proxyAddr := flag.String("teller-proxy-addr", "0.0.0.0:7070", "teller proxy listen address")
	httpAddr := flag.String("http-service-addr", "localhost:7071", "http api service address")
	newKey := flag.Bool("new-key", false, "generate new key pairs")
	flag.Parse()

	log := logger.NewLogger("", false)

	appDir, err := createAppDirIfNotExist(appDir)
	if err != nil {
		log.Println("Create app dir failed:", err)
		return
	}

	// start gops agent, for profilling
	if err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	}); err != nil {
		log.Println("Start profile agent failed:", err)
		return
	}

	pkPath := filepath.Join(appDir, privateKeyFile)
	var lseckey cipher.SecKey
	secKey := loadPrivatekey(pkPath)
	if secKey != nil && !(*newKey) {
		lseckey, err = cipher.SecKeyFromHex(secKey.PrivateKey)
		if err != nil {
			log.Println("Load seckey from hex string failed:", err)
			return
		}

		pubkey := cipher.PubKeyFromSecKey(lseckey)
		log.Println("Pubkey:", pubkey.Hex())
	} else {
		_, lseckey = cipher.GenerateKeyPair()
		pubkey := cipher.PubKeyFromSecKey(lseckey).Hex()
		log.Println("Pubkey:", pubkey)
		if err := persistPrivateKey(lseckey.Hex(), pkPath); err != nil {
			log.Println("Persist pubkey failed:", err)
			return
		}
	}

	auth := &daemon.Auth{
		LSeckey: lseckey,
	}

	px := proxy.New(*proxyAddr, *httpAddr, auth, proxy.Logger(log))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		px.Run()
	}()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	log.Println("Shutting down...")

	px.Shutdown()
	wg.Wait()
	log.Println("Shutdown complete")
}

func createAppDirIfNotExist(app string) (string, error) {
	cur, err := user.Current()
	if err != nil {
		return "", err
	}
	path := filepath.Join(cur.HomeDir, app)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// create the dir
		if err := os.Mkdir(path, 0700); err != nil {
			return "", err
		}
	}
	return path, nil
}

func loadPrivatekey(pkfile string) *privateKey {
	if _, err := os.Stat(pkfile); os.IsNotExist(err) {
		return nil
	}

	v, err := ioutil.ReadFile(pkfile)
	if err != nil {
		fmt.Println("Read pubkey file failed:", err)
		return nil
	}
	var pk privateKey
	if err := json.NewDecoder(bytes.NewReader(v)).Decode(&pk); err != nil {
		fmt.Println("Decode pubkey value failed:", err)
		return nil
	}
	return &pk
}

func persistPrivateKey(key string, path string) error {
	v, err := json.MarshalIndent(privateKey{PrivateKey: key}, "", "    ")
	if err != nil {
		return fmt.Errorf("encode pubkeyValue failed: %v", err)
	}

	if err := ioutil.WriteFile(path, v, 0700); err != nil {
		return fmt.Errorf("write pubkey to file failed: %v", err)
	}

	return nil
}
