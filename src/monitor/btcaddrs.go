package monitor

import (
	"encoding/json"
	_ "encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/util/httputil"
)

type BtcAddressJSON struct {
	Addresses []string `json:"btc_addresses"`
}

type CountBtc struct {
	All  int
	Used int
	Free int
}

// method: POST
// url: /addbtc  {"btc_addresses": ["1st_add", "2nd_adr", ...]}
var AddBtcAddresses = func(w http.ResponseWriter, r *http.Request) {
	_, err := addrs.NewBTCAddrs(logrus.WithField("prefix", "addrs"), database, r.Body)
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}
	fmt.Fprintf(w, "added.")
}

// method: GET
// url: /getbtc
var GetBtcAddresses = func(w http.ResponseWriter, r *http.Request) {
	log.Println("show all.")
	var btc_adr BtcAddressJSON

	manager, err := addrs.GetBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	btc_adr.Addresses, err = manager.GetAll()
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	httputil.JSONResponse(w, btc_adr)
}

// method: GET
// url: /getusedbtc
var GetBtcUsedAddresses = func(w http.ResponseWriter, r *http.Request) {
	log.Println("show used.")
	var btc_adr BtcAddressJSON

	manager, err := addrs.GetBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	btc_adr.Addresses, err = manager.GetUsed()
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	httputil.JSONResponse(w, btc_adr)
}

// method: GET
// url: /allfreebtc
var GetAllFreeBTC = func(w http.ResponseWriter, r *http.Request) {
	log.Println("show free.")
	var btc_adr BtcAddressJSON

	manager, err := addrs.GetBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	btc_adr.Addresses, err = manager.GetFree()
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	httputil.JSONResponse(w, btc_adr)
}

// method: POST
// url: /setusedbtc  {"btc_addresses": ["1st_add", "2nd_adr", ...]}
var SetUsedBtc = func(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var adr BtcAddressJSON
	err := decoder.Decode(&adr)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	manager, err := addrs.GetBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	for _, btcAddress := range adr.Addresses {
		err = manager.SetUsed(btcAddress)
		if err != nil {
			fmt.Fprintf(w, "failed.")
		}
	}
	fmt.Fprintf(w, "set used.")
}

// method: GET
// url: /getcountbtc
var GetCountBTC = func(w http.ResponseWriter, r *http.Request) {
	log.Println("show free.")
	var count CountBtc

	manager, err := addrs.GetBTCManager(logrus.WithField("prefix", "addrs"), database)
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}

	free, err := manager.GetFree()
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}
	count.Free = len(free)

	used, err := manager.GetUsed()
	if err != nil {
		fmt.Fprintf(w, "failed.")
	}
	count.Used = len(used)

	count.All = count.Free + count.Used

	httputil.JSONResponse(w, count)
}
