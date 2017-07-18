// This is the ico-server, which provides service of monitoring the deposite bitcoin
// addresses and sending coins to specific skycoin addresses.
// Note: althought this is a server end, but it's actually run
// in the private enverioment for security.
package main

import (
	"log"
	"os/signal"
	"os/user"
	"time"

	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/google/gops/agent"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service"
	"github.com/skycoin/teller/src/service/config"

	"fmt"
	"os"
)

const (
	RemotePubkey = "03554c1787c0d49ddbced3b9d4f9f1163c01f5d1bcf52ae7362a63027e1a896cef"
	LocalSeckey  = "81ea5dbfa6fab837bbcb96480cff49b57e7135a15e7d0a6d021177edecf246d3"

	appDir = ".skycoin-teller"
	dbName = "data.db"
)

func main() {
	// generate auth key pairs
	// for i := 0; i < 2; i++ {
	// 	pub, sec := cipher.GenerateKeyPair()
	// 	fmt.Println(pub.Hex())
	// 	fmt.Println(sec.Hex())
	// }

	// start gops agent, for profilling
	if err := agent.Listen(&agent.Options{
		NoShutdownCleanup: true,
	}); err != nil {
		log.Fatal(err)
	}

	// load config
	cfg, err := config.New("config.json")
	if err != nil {
		panic(err)
	}

	rpubkey := cipher.MustPubKeyFromHex(RemotePubkey)
	lseckey := cipher.MustSecKeyFromHex(LocalSeckey)

	// mux := net.NewMux()
	appDir, err := createAppDirIfNotExist()
	if err != nil {
		panic(err)
	}

	// db path
	dbPath := filepath.Join(appDir, dbName)
	db, err := bolt.Open(dbPath, 0700, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		panic(fmt.Sprintf("Open db failed, err: %v", err))
	}

	// init logger
	log := logger.NewLogger("", true)

	// prepare auth
	auth := &daemon.Auth{
		RPubkey: rpubkey,
		LSeckey: lseckey,
	}

	srv, stop := service.New(cfg.ProxyAddress, auth, db,
		service.Logger(log),
		service.DialTimeout(cfg.DialTimeout*time.Second),
		service.ReconnectTime(cfg.ReconnectTime*time.Second),
		service.PingTimeout(cfg.PingTimeout*time.Second),
		service.PongTimeout(cfg.PongTimeout*time.Second),
		service.NodeRPCAddr(cfg.Node.RPCAddress),
		service.NodeWalletPath(cfg.Node.WalletPath),
		service.MonitorCheckPeriod(cfg.Monitor.CheckPeriod*time.Second),
		service.ExchangeRateTable(cfg.ExchangeRate),
		service.DepositCoin(cfg.DepositCoin),
		service.ICOCoin(cfg.ICOCoin),
	)

	// Run the service
	srv.Run()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	signal.Stop(sigchan)
	stop()
}

func createAppDirIfNotExist() (string, error) {
	cur, err := user.Current()
	if err != nil {
		return "", err
	}
	path := filepath.Join(cur.HomeDir, appDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// create the dir
		if err := os.Mkdir(path, 0700); err != nil {
			return "", err
		}
	}
	return path, nil
}
