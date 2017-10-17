package server

import (
	_"encoding/json"
	"fmt"
	_"log"
	"net/http"
    "github.com/skycoin/teller/src/addrs"
	"github.com/sirupsen/logrus"
	_"github.com/skycoin/teller/src/util/testutil"
)

// method: POST
// url: /addbtc  {"btc_addresses": ["1st_add", "2nd_adr", ...]}
var AddBtcAddresses = func(w http.ResponseWriter, r *http.Request) {
 	a, _ :=	addrs.NewBTCAddrs( logrus.WithField("prefix", "addrs"), database, r.Body )
	//s, _ := adr.NewAddress()
	s,_ := a.NewAddress()
	log.Println("s=", s)
	fmt.Fprintf(w, "added.")
}
