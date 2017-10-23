package monitor

import (
	"encoding/json"
	_ "encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/util/httputil"
)

type BtcAddressJSON struct {
	Addresses []string `json:"btc_addresses"`
}

type CountBtc struct {
	All  int `json:"all"`
	Used int `json:"used"`
	Free int `json:"free"`
}

func openDB() (*bolt.DB, error) {
	db, err := bolt.Open("test.db", 0600, nil)
	if err != nil {
		return nil, err
	}
	return db, err
}

// method: POST
// url: /addbtc  {"btc_addresses": ["1st_add", "2nd_adr", ...]}
func AddBtcAddresses(w http.ResponseWriter, r *http.Request) {
	database, e := openDB()
	if e != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	defer database.Close()

	_, err := addrs.NewBTCAddrs(logrus.WithField("prefix", "addrs"), database, r.Body)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't put into db")
		return
	}
	fmt.Fprintf(w, "added.")
}

// method: GET
// url: /getbtc
func GetBtcAddresses(w http.ResponseWriter, r *http.Request) {
	log.Println("show all.")
	var btcAddrs BtcAddressJSON

	database, e := openDB()
	if e != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	defer database.Close()

	manager, err := addrs.NewBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	btcAddrs.Addresses, err = manager.GetAll()
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	httputil.JSONResponse(w, btcAddrs)
}

// method: GET
// url: /getusedbtc
func GetBtcUsedAddresses(w http.ResponseWriter, r *http.Request) {
	log.Println("show used.")
	var btcAddrs BtcAddressJSON

	database, e := openDB()
	if e != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	defer database.Close()

	manager, err := addrs.NewBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	btcAddrs.Addresses, err = manager.GetUsed()
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	httputil.JSONResponse(w, btcAddrs)
}

// method: GET
// url: /allfreebtc
func GetAllFreeBTC(w http.ResponseWriter, r *http.Request) {
	log.Println("show free.")
	var btc_adr BtcAddressJSON

	database, e := openDB()
	if e != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	defer database.Close()

	manager, err := addrs.NewBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	btc_adr.Addresses, err = manager.GetFree()
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	httputil.JSONResponse(w, btc_adr)
}

// method: POST
// url: /setusedbtc  {"btc_addresses": ["1st_add", "2nd_adr", ...]}
func SetUsedBtc(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var adr BtcAddressJSON
	err := decoder.Decode(&adr)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	database, e := openDB()
	if e != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	defer database.Close()

	manager, err := addrs.NewBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	for _, btcAddress := range adr.Addresses {
		err = manager.SetUsed(btcAddress)
		if err != nil {
			httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
			return
		}
	}
	fmt.Fprintf(w, "set used.")
}

// method: GET
// url: /getcountbtc
func GetCountBTC(w http.ResponseWriter, r *http.Request) {
	log.Println("show free.")
	var count CountBtc

	database, e := openDB()
	if e != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	defer database.Close()

	manager, err := addrs.NewBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}

	free, err := manager.GetFree()
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	count.Free = len(free)

	used, err := manager.GetUsed()
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, "Can't access to db")
		return
	}
	count.Used = len(used)

	count.All = count.Free + count.Used

	httputil.JSONResponse(w, count)
}
