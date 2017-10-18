package monitor

import (
	"errors"
	"net/http"

	"github.com/boltdb/bolt"
	"github.com/rs/cors"
)

var database *bolt.DB

func LaunchServer(port string) {
	var err error
	database, err = bolt.Open("test.db", 0600, nil)
	if err != nil {
		errors.New("Can't open database")
	}
	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir("../static/dist")))
	mux.HandleFunc("/addbtc", AddBtcAddresses)
	mux.HandleFunc("/getbtc", GetBtcAddresses)
	mux.HandleFunc("/getusedbtc", GetBtcUsedAddresses)
	mux.HandleFunc("/setusedbtc", SetUsedBtc)
	mux.HandleFunc("/allfreebtc", GetAllFreeBTC)
	mux.HandleFunc("/getcountbtc", GetCountBTC)

	handler := cors.Default().Handler(mux)
	http.ListenAndServe(":"+port, handler)
}
